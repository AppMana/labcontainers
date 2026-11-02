package client

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	labv1 "github.com/appmana/labcontainers/api/v1"
	clab "github.com/appmana/labcontainers/pkg/containerlab"
	"github.com/srl-labs/containerlab/core"
	"github.com/srl-labs/containerlab/links"
	"github.com/srl-labs/containerlab/types"
)

// TestLiveRedisSentinelDeadlockAndRecovery models the exact production mutual
// deadlock observed in spellsource (stale sentinel topology accumulation +
// simultaneous master pod and sentinel pod restart with new Pod IPs where
// surviving replicas hold high repl_offset pointing to the dead master IP,
// the restarted master pod boots empty at repl_offset=0, and sentinel-0 boots
// monitoring 0.0.0.0:6379 while sentinel-1/sentinel-2 abort failover forever
// with -failover-abort-not-elected), and verifies the redis-operator fix
// end-to-end against real Redis and Sentinel containers in labcontainers.
func TestLiveRedisSentinelDeadlockAndRecovery(t *testing.T) {
	if os.Getenv("LABCONTAINERS_LIVE") == "" {
		t.Skip("set LABCONTAINERS_LIVE=1 to run the privileged labcontainers reproduction and recovery test")
	}

	root, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	operatorRepo := filepath.Join(filepath.Dir(root), "forks-redis-operator-cold-start")
	if _, err := os.Stat(operatorRepo); err != nil {
		t.Fatalf("operator repo not found at %s: %v", operatorRepo, err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	// 1. Compile the static operator reconciler test binary from forks-redis-operator-cold-start.
	testBin := filepath.Join(t.TempDir(), "redisreplication.test")
	buildCmd := exec.CommandContext(ctx, "go", "test", "-c", "-o", testBin, "./internal/controller/redisreplication")
	buildCmd.Dir = operatorRepo
	buildCmd.Env = append(os.Environ(), "CGO_ENABLED=0")
	if out, err := buildCmd.CombinedOutput(); err != nil {
		t.Fatalf("compile operator test binary failed: %v\n%s", err, out)
	}
	if err := os.Chmod(testBin, 0o755); err != nil {
		t.Fatal(err)
	}

	// 2. Launch labcontainers session with isolated L2 switch bridge and Redis/Sentinel nodes.
	labdPath := os.Getenv("LABCONTAINERS_LABD")
	if labdPath == "" {
		labdPath = filepath.Join(root, "bin", "labd")
	}
	c, err := Launch(ctx, Options{LabdPath: labdPath})
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := c.Close(); err != nil {
			t.Errorf("close: %v", err)
		}
	}()

	prefix := "redis-deadlock"
	topology, err := clab.Source(&core.Config{
		Name:   "spellsource",
		Prefix: &prefix,
		Topology: &types.Topology{
			Defaults: &types.NodeDefinition{
				Kind:        "linux",
				NetworkMode: "none",
				User:        "root",
				Entrypoint:  "/bin/sh",
				Cmd:         "-c 'sleep 3600'",
			},
			Nodes: map[string]*types.NodeDefinition{
				"switch":         {Image: "alpine:3.20"},
				"redis-master-0": {Image: "quay.io/opstree/redis:v7.0.15"},
				"redis-master-1": {Image: "quay.io/opstree/redis:v7.0.15"},
				"redis-master-2": {Image: "quay.io/opstree/redis:v7.0.15"},
				"sentinel-0":     {Image: "quay.io/opstree/redis-sentinel:v7.0.15"},
				"sentinel-1":     {Image: "quay.io/opstree/redis-sentinel:v7.0.15"},
				"sentinel-2":     {Image: "quay.io/opstree/redis-sentinel:v7.0.15"},
				"ghost-sentinel": {Image: "quay.io/opstree/redis-sentinel:v7.0.15"},
				"operator": {
					Image: "alpine:3.20",
					Binds: []string{testBin + ":/usr/local/bin/redisreplication.test:ro"},
				},
			},
			Links: []*links.LinkDefinition{
				{Link: &links.LinkBriefRaw{Endpoints: []string{"redis-master-0:eth1", "switch:eth1"}}},
				{Link: &links.LinkBriefRaw{Endpoints: []string{"redis-master-1:eth1", "switch:eth2"}}},
				{Link: &links.LinkBriefRaw{Endpoints: []string{"redis-master-2:eth1", "switch:eth3"}}},
				{Link: &links.LinkBriefRaw{Endpoints: []string{"sentinel-0:eth1", "switch:eth4"}}},
				{Link: &links.LinkBriefRaw{Endpoints: []string{"sentinel-1:eth1", "switch:eth5"}}},
				{Link: &links.LinkBriefRaw{Endpoints: []string{"sentinel-2:eth1", "switch:eth6"}}},
				{Link: &links.LinkBriefRaw{Endpoints: []string{"ghost-sentinel:eth1", "switch:eth7"}}},
				{Link: &links.LinkBriefRaw{Endpoints: []string{"operator:eth1", "switch:eth8"}}},
			},
		},
	})
	if err != nil {
		t.Fatal(err)
	}

	lab, err := c.Start(ctx, &labv1.LabSpec{Topology: topology}, 5*time.Minute)
	if err != nil {
		t.Fatal(err)
	}

	execOK := func(node string, script string) string {
		t.Helper()
		res, err := lab.Node(node).Exec(ctx, "sh", "-ec", script)
		if err != nil {
			t.Fatalf("%s exec error: %v", node, err)
		}
		if res.GetExitCode() != 0 {
			t.Fatalf("%s command failed (exit %d):\nstdout: %s\nstderr: %s",
				node, res.GetExitCode(), res.GetStdout(), res.GetStderr())
		}
		return strings.TrimSpace(string(res.GetStdout()))
	}

	// 3. Configure L2 bridge on switch and initial Pod IPs on all nodes.
	execOK("switch", `
ip link add br0 type bridge
for i in eth1 eth2 eth3 eth4 eth5 eth6 eth7 eth8; do
  ip link set "$i" master br0
  ip link set "$i" up
done
ip link set br0 up
`)

	execOK("redis-master-0", "ip addr add 10.3.104.38/16 dev eth1 && ip link set eth1 up")
	execOK("redis-master-1", "ip addr add 10.3.22.21/16 dev eth1 && ip link set eth1 up")
	execOK("redis-master-2", "ip addr add 10.3.239.142/16 dev eth1 && ip link set eth1 up")
	execOK("sentinel-0", "ip addr add 10.3.100.10/16 dev eth1 && ip link set eth1 up")
	execOK("sentinel-1", "ip addr add 10.3.100.11/16 dev eth1 && ip link set eth1 up")
	execOK("sentinel-2", "ip addr add 10.3.100.12/16 dev eth1 && ip link set eth1 up")
	execOK("ghost-sentinel", "ip addr add 10.3.200.91/16 dev eth1 && ip link set eth1 up")
	execOK("operator", "ip addr add 10.3.0.1/16 dev eth1 && ip link set eth1 up")

	// 4. Phase 1: Boot initial healthy 3-pod RedisReplication (redis-master-2 as master)
	// and 3-pod RedisSentinel + 1 ghost sentinel (representing an earlier un-retired sentinel restart).
	for _, node := range []string{"redis-master-0", "redis-master-1", "redis-master-2"} {
		execOK(node, `
kill -9 $(pidof redis-server) 2>/dev/null || true
rm -rf /tmp/redis
mkdir -p /tmp/redis
cat >/tmp/redis/redis.conf <<'EOF'
bind 0.0.0.0
port 6379
protected-mode no
repl-timeout 2
dir /tmp/redis
appendonly no
save ""
EOF
redis-server /tmp/redis/redis.conf --daemonize yes
`)
	}
	execOK("redis-master-0", "redis-cli -h 127.0.0.1 -p 6379 REPLICAOF 10.3.239.142 6379")
	execOK("redis-master-1", "redis-cli -h 127.0.0.1 -p 6379 REPLICAOF 10.3.239.142 6379")

	// Write dataset to initial master redis-master-2 (10.3.239.142) and wait for replication.
	execOK("redis-master-2", `
redis-cli -h 127.0.0.1 -p 6379 SET spellsource:card:1 fireball
redis-cli -h 127.0.0.1 -p 6379 SET spellsource:card:2 polymorph
`)
	// Wait for both replicas to replicate the 2 keys.
	for _, replica := range []string{"redis-master-0", "redis-master-1"} {
		execOK(replica, `
for i in $(seq 1 30); do
  if [ "$(redis-cli -h 127.0.0.1 -p 6379 DBSIZE)" = "2" ]; then exit 0; fi
  sleep 0.2
done
exit 1
`)
	}

	startSentinel := func(node, ip, masterIP string) {
		execOK(node, fmt.Sprintf(`
kill -9 $(pidof redis-sentinel) 2>/dev/null || true
rm -rf /tmp/sentinel
mkdir -p /tmp/sentinel
cat >/tmp/sentinel/sentinel.conf <<'EOF'
port 26379
bind 0.0.0.0
protected-mode no
dir /tmp/sentinel
sentinel announce-ip %s
sentinel announce-port 26379
sentinel monitor myMaster %s 6379 2
sentinel down-after-milliseconds myMaster 2000
sentinel failover-timeout myMaster 4000
sentinel parallel-syncs myMaster 1
EOF
redis-sentinel /tmp/sentinel/sentinel.conf --logfile /tmp/sentinel/sentinel.log --daemonize yes
`, ip, masterIP))
	}

	startSentinel("sentinel-0", "10.3.100.10", "10.3.239.142")
	startSentinel("sentinel-1", "10.3.100.11", "10.3.239.142")
	startSentinel("sentinel-2", "10.3.100.12", "10.3.239.142")
	startSentinel("ghost-sentinel", "10.3.200.91", "10.3.239.142")

	// Wait until sentinel-1 and sentinel-2 see all 4 sentinels (num-other-sentinels: 3).
	for _, sNode := range []string{"sentinel-1", "sentinel-2"} {
		execOK(sNode, `
for i in $(seq 1 40); do
  if redis-cli -p 26379 INFO sentinel | grep -q "sentinels=4"; then exit 0; fi
  sleep 0.25
done
redis-cli -p 26379 INFO sentinel >&2
exit 1
`)
	}

	// Retire ghost-sentinel process (simulating an earlier sentinel pod restart whose old IP 10.3.200.91 was never reset).
	execOK("ghost-sentinel", "kill -9 $(pidof redis-sentinel) 2>/dev/null || true; ip addr flush dev eth1")

	// Give redis-master-1 a strictly higher replication offset before redis-master-2 dies:
	// cut switch port eth1 (redis-master-0) using native labcontainers SetLink, write key 3 on
	// redis-master-2 so only redis-master-1 receives it.
	linkFault, err := lab.SetLink(ctx, "switch", "eth1", false)
	if err != nil {
		t.Fatal(err)
	}
	execOK("redis-master-2", "redis-cli -h 127.0.0.1 -p 6379 SET spellsource:card:3 meteor")
	execOK("redis-master-1", `
for i in $(seq 1 30); do
  if [ "$(redis-cli -h 127.0.0.1 -p 6379 DBSIZE)" = "3" ]; then exit 0; fi
  sleep 0.2
done
exit 1
`)

	// 5. Phase 2: Simultaneous restart of master pod (redis-master-2: 10.3.239.142 -> 10.3.174.183, empty role:master)
	// and sentinel-0 (10.3.100.10 -> 10.3.100.20, monitoring placeholder 0.0.0.0:6379).
	execOK("redis-master-2", `
kill -9 $(pidof redis-server) 2>/dev/null || true
rm -rf /tmp/redis/*
ip addr del 10.3.239.142/16 dev eth1
ip addr add 10.3.174.183/16 dev eth1
cat >/tmp/redis/redis.conf <<'EOF'
bind 0.0.0.0
port 6379
protected-mode no
repl-timeout 2
dir /tmp/redis
appendonly no
save ""
EOF
redis-server /tmp/redis/redis.conf --daemonize yes
`)
	if err := linkFault.Revert(ctx); err != nil {
		t.Fatal(err)
	}

	execOK("sentinel-0", `
kill -9 $(pidof redis-sentinel) 2>/dev/null || true
rm -rf /tmp/sentinel/*
ip addr del 10.3.100.10/16 dev eth1
ip addr add 10.3.100.20/16 dev eth1
cat >/tmp/sentinel/sentinel.conf <<'EOF'
port 26379
bind 0.0.0.0
protected-mode no
dir /tmp/sentinel
sentinel announce-ip 10.3.100.20
sentinel announce-port 26379
sentinel monitor myMaster 0.0.0.0 6379 2
sentinel down-after-milliseconds myMaster 2000
sentinel failover-timeout myMaster 4000
sentinel parallel-syncs myMaster 1
EOF
redis-sentinel /tmp/sentinel/sentinel.conf --logfile /tmp/sentinel/sentinel.log --daemonize yes
`)

	// 6. Verify that Sentinel enters the permanent mutual deadlock:
	// sentinel-1 and sentinel-2 detect +odown on 10.3.239.142 (quorum 2 met) and try failover,
	// but abort with -failover-abort-not-elected because 4 total sentinels are known
	// (2 live + 2 dead IPs 10.3.100.10 and 10.3.200.91 -> majority 3 required, only 2 votes cast)!
	deadlockConfirmed := false
	for attempt := 0; attempt < 30; attempt++ {
		log1 := execOK("sentinel-1", "cat /tmp/sentinel/sentinel.log")
		log2 := execOK("sentinel-2", "cat /tmp/sentinel/sentinel.log")
		if strings.Contains(log1, "-failover-abort-not-elected") || strings.Contains(log2, "-failover-abort-not-elected") {
			t.Logf("Confirmed Sentinel leader-election deadlock in live logs:\nsentinel-1:\n%s\nsentinel-2:\n%s", log1, log2)
			deadlockConfirmed = true
			break
		}
		time.Sleep(500 * time.Millisecond)
	}
	if !deadlockConfirmed {
		log1 := execOK("sentinel-1", "cat /tmp/sentinel/sentinel.log")
		log2 := execOK("sentinel-2", "cat /tmp/sentinel/sentinel.log")
		t.Fatalf("expected -failover-abort-not-elected in sentinel logs:\nsentinel-1:\n%s\nsentinel-2:\n%s", log1, log2)
	}

	// 7. Phase 3: Run the fixed operator reconcilers inside the "operator" container and verify recovery!
	out := execOK("operator", "LABCONTAINERS_IN_NETNS=1 /usr/local/bin/redisreplication.test -test.v -test.run '^TestLabcontainersLiveReconcileEntrypoint$'")
	t.Logf("Operator reconcile output:\n%s", out)
}
