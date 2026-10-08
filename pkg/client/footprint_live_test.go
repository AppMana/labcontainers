package client

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	labv1 "github.com/appmana/labcontainers/api/v1"
	clab "github.com/appmana/labcontainers/pkg/containerlab"
	"github.com/srl-labs/containerlab/core"
	"github.com/srl-labs/containerlab/types"
)

// vmFootprint is what one disposable test VM costs its host.
type vmFootprint struct {
	Node string `json:"node"`
	// Seconds from the start of deployment.
	QGAReadySeconds      float64 `json:"qga_ready_seconds"`
	CloudInitDoneSeconds float64 `json:"cloud_init_done_seconds"`
	SystemdAnalyze       string  `json:"systemd_analyze"`
	GuestMemTotalKiB     int64   `json:"guest_mem_total_kib"`
	QEMUArgs             string  `json:"qemu_args"`
	WrapperLog           string  `json:"wrapper_log"`
	BootedRSSKiB         int64   `json:"booted_rss_kib"`
	AfterTransientRSSKiB int64   `json:"after_transient_rss_kib"`
	// Every process in the wrapper container, QEMU included.
	ContainerRSSKiB       int64    `json:"container_rss_kib"`
	ContainerProcesses    []string `json:"container_processes"`
	TransientAllocatedMiB int      `json:"transient_allocated_mib"`
}

type footprintReport struct {
	Image          string        `json:"image"`
	Env            []string      `json:"env"`
	DeploySeconds  float64       `json:"deploy_seconds"`
	VMs            []vmFootprint `json:"vms"`
	HostMemTotalKB int64         `json:"host_mem_total_kib"`
}

// TestLiveVMFootprint boots LABCONTAINERS_FOOTPRINT_VMS (default 1) Linux
// test VMs with no NICs and records, per VM: time to QGA and to cloud-init
// completion, the guest's own boot breakdown, and the host RSS of its QEMU
// process after boot and after the guest allocated then released memory.
// QEMU_* variables from LABCONTAINERS_FOOTPRINT_ENV (comma separated) are
// passed to each node unchanged so profiles can be compared on one host.
func TestLiveVMFootprint(t *testing.T) {
	if os.Getenv("LABCONTAINERS_FOOTPRINT_LIVE") == "" {
		t.Skip("set LABCONTAINERS_FOOTPRINT_LIVE=1 to measure VM boot time and memory")
	}
	root, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	labdPath := os.Getenv("LABCONTAINERS_LABD")
	if labdPath == "" {
		labdPath = filepath.Join(root, "bin", "labd")
	}
	image := os.Getenv("LABCONTAINERS_VM_IMAGE")
	if image == "" {
		image = "labcontainers/vm-ubuntu:jammy"
	}
	count := 1
	if v := os.Getenv("LABCONTAINERS_FOOTPRINT_VMS"); v != "" {
		if count, err = strconv.Atoi(v); err != nil || count < 1 {
			t.Fatalf("bad LABCONTAINERS_FOOTPRINT_VMS %q", v)
		}
	}
	nodeEnv := map[string]string{}
	var envList []string
	if v := os.Getenv("LABCONTAINERS_FOOTPRINT_ENV"); v != "" {
		for _, kv := range strings.Split(v, ",") {
			k, val, ok := strings.Cut(kv, "=")
			if !ok {
				t.Fatalf("bad LABCONTAINERS_FOOTPRINT_ENV entry %q", kv)
			}
			nodeEnv[k] = val
			envList = append(envList, kv)
		}
	}
	transientMiB := 256

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Minute)
	defer cancel()
	c, err := Launch(ctx, Options{LabdPath: labdPath})
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := c.Close(); err != nil {
			t.Errorf("close: %v", err)
		}
	}()
	nodes := map[string]*types.NodeDefinition{}
	extensions := map[string]*labv1.NodeExtension{}
	var names []string
	for i := 1; i <= count; i++ {
		name := fmt.Sprintf("vm%d", i)
		names = append(names, name)
		nodes[name] = &types.NodeDefinition{Kind: "generic_vm", Image: image, NetworkMode: "none", ImagePullPolicy: "Never", Env: nodeEnv}
		extensions[name] = &labv1.NodeExtension{Control: "qga"}
	}
	topology, err := clab.Source(&core.Config{Name: "footprint", Topology: &types.Topology{Nodes: nodes}})
	if err != nil {
		t.Fatal(err)
	}
	began := time.Now()
	lab, err := c.Start(ctx, &labv1.LabSpec{Topology: topology, Nodes: extensions}, 20*time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	report := footprintReport{Image: image, Env: envList, DeploySeconds: time.Since(began).Seconds(), HostMemTotalKB: meminfo(t, "MemTotal")}

	results := make([]vmFootprint, len(names))
	var wg sync.WaitGroup
	errs := make(chan error, len(names))
	for i, name := range names {
		wg.Add(1)
		go func(i int, name string) {
			defer wg.Done()
			r, err := measureVM(ctx, lab, name, began, transientMiB)
			results[i] = r
			if err != nil {
				errs <- fmt.Errorf("%s: %w", name, err)
			}
		}(i, name)
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		t.Error(err)
	}
	report.VMs = results
	encoded, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	out := filepath.Join(lab.Artifacts(), "footprint.json")
	if err := os.WriteFile(out, encoded, 0o600); err != nil {
		t.Fatal(err)
	}
	t.Logf("footprint report: %s", out)
	var slowest float64
	var rss int64
	for _, r := range results {
		if r.CloudInitDoneSeconds > slowest {
			slowest = r.CloudInitDoneSeconds
		}
		rss += r.ContainerRSSKiB
	}
	t.Logf("%d VMs: all booted by %.1fs; wrapper containers hold %d MiB in total", len(results), slowest, rss/1024)
	for _, r := range results {
		t.Logf("%s: qga %.1fs, cloud-init %.1fs, guest %d MiB, booted RSS %d MiB, RSS after %d MiB transient %d MiB",
			r.Node, r.QGAReadySeconds, r.CloudInitDoneSeconds, r.GuestMemTotalKiB/1024, r.BootedRSSKiB/1024, r.TransientAllocatedMiB, r.AfterTransientRSSKiB/1024)
	}
}

func measureVM(ctx context.Context, lab *Session, name string, began time.Time, transientMiB int) (vmFootprint, error) {
	r := vmFootprint{Node: name, TransientAllocatedMiB: transientMiB}
	node := lab.Node(name)
	for {
		res, err := node.ExecWithTimeout(ctx, 5*time.Second, "true")
		if err == nil && res.GetExitCode() == 0 {
			break
		}
		if ctx.Err() != nil {
			return r, fmt.Errorf("QGA never answered: %v", err)
		}
		time.Sleep(250 * time.Millisecond)
	}
	r.QGAReadySeconds = time.Since(began).Seconds()
	res, err := node.ExecWithTimeout(ctx, 10*time.Minute, "cloud-init", "status", "--wait")
	if err != nil {
		return r, err
	}
	if res.GetExitCode() != 0 {
		return r, fmt.Errorf("cloud-init status exited %d: %s%s", res.GetExitCode(), res.GetStdout(), res.GetStderr())
	}
	r.CloudInitDoneSeconds = time.Since(began).Seconds()
	res, err = node.ExecWithTimeout(ctx, time.Minute, "sh", "-c", "systemd-analyze; systemd-analyze blame | head -n 15; grep MemTotal /proc/meminfo")
	if err != nil {
		return r, err
	}
	r.SystemdAnalyze = string(res.GetStdout())
	for _, line := range strings.Split(r.SystemdAnalyze, "\n") {
		if f := strings.Fields(line); len(f) >= 2 && f[0] == "MemTotal:" {
			r.GuestMemTotalKiB, _ = strconv.ParseInt(f[1], 10, 64)
		}
	}
	pid, args, err := qemuProcess(ctx, lab.ID(), name)
	if err != nil {
		return r, err
	}
	r.QEMUArgs = args
	if log, err := wrapperLog(ctx, lab.ID(), name); err == nil {
		r.WrapperLog = log
	}
	// Let boot-time page reporting and writeback settle before sampling.
	time.Sleep(15 * time.Second)
	if r.BootedRSSKiB, err = processRSS(pid); err != nil {
		return r, err
	}
	// Fault memory in, then free it: a host that gets free pages back sees
	// RSS return near the booted value; one that does not keeps it.
	res, err = node.ExecWithTimeout(ctx, 2*time.Minute, "sh", "-ec", fmt.Sprintf("mkdir -p /run/footprint; mount -t tmpfs -o size=%dm tmpfs /run/footprint; dd if=/dev/urandom of=/run/footprint/fill bs=1M count=%d status=none; rm /run/footprint/fill; umount /run/footprint", transientMiB+16, transientMiB))
	if err != nil {
		return r, err
	}
	if res.GetExitCode() != 0 {
		return r, fmt.Errorf("transient allocation exited %d: %s", res.GetExitCode(), res.GetStderr())
	}
	time.Sleep(15 * time.Second)
	if r.AfterTransientRSSKiB, err = processRSS(pid); err != nil {
		return r, err
	}
	r.ContainerRSSKiB, r.ContainerProcesses, err = containerRSS(ctx, lab.ID(), name)
	return r, err
}

func containerRSS(ctx context.Context, session, node string) (int64, []string, error) {
	out, err := exec.CommandContext(ctx, "docker", "ps", "--filter", "label=labcontainers.appmana.com/session="+session, "--filter", "label=clab-node-name="+node, "--format", "{{.ID}}").Output()
	if err != nil {
		return 0, nil, err
	}
	ids := strings.Fields(string(out))
	if len(ids) != 1 {
		return 0, nil, fmt.Errorf("expected one container for %s, found %q", node, ids)
	}
	out, err = exec.CommandContext(ctx, "docker", "top", ids[0], "-eo", "pid,args").Output()
	if err != nil {
		return 0, nil, err
	}
	var total int64
	var processes []string
	for _, line := range strings.Split(string(out), "\n")[1:] {
		f := strings.Fields(line)
		if len(f) < 2 {
			continue
		}
		pid, err := strconv.Atoi(f[0])
		if err != nil {
			continue
		}
		rss, err := processRSS(pid)
		if err != nil {
			return 0, nil, err
		}
		total += rss
		args := strings.Join(f[1:], " ")
		if len(args) > 80 {
			args = args[:80]
		}
		processes = append(processes, fmt.Sprintf("%d KiB %s", rss, args))
	}
	return total, processes, nil
}

// qemuProcess finds the node's QEMU through the session ownership label.
func qemuProcess(ctx context.Context, session, node string) (int, string, error) {
	out, err := exec.CommandContext(ctx, "docker", "ps", "--filter", "label=labcontainers.appmana.com/session="+session, "--filter", "label=clab-node-name="+node, "--format", "{{.ID}}").Output()
	if err != nil {
		return 0, "", err
	}
	ids := strings.Fields(string(out))
	if len(ids) != 1 {
		return 0, "", fmt.Errorf("expected one container for %s, found %q", node, ids)
	}
	out, err = exec.CommandContext(ctx, "docker", "top", ids[0], "-eo", "pid,args").Output()
	if err != nil {
		return 0, "", err
	}
	var found []string
	scanner := bufio.NewScanner(strings.NewReader(string(out)))
	for scanner.Scan() {
		if f := strings.Fields(scanner.Text()); len(f) > 1 && strings.HasPrefix(filepath.Base(f[1]), "qemu-system-") {
			found = append(found, scanner.Text())
		}
	}
	sort.Strings(found)
	if len(found) != 1 {
		return 0, "", fmt.Errorf("expected one QEMU process in %s, found %d", node, len(found))
	}
	f := strings.Fields(found[0])
	pid, err := strconv.Atoi(f[0])
	return pid, strings.Join(f[1:], " "), err
}

// wrapperLog keeps the launcher's timestamped output, which shows how long
// the wrapper took before QEMU started.
func wrapperLog(ctx context.Context, session, node string) (string, error) {
	out, err := exec.CommandContext(ctx, "docker", "ps", "--filter", "label=labcontainers.appmana.com/session="+session, "--filter", "label=clab-node-name="+node, "--format", "{{.ID}}").Output()
	if err != nil {
		return "", err
	}
	ids := strings.Fields(string(out))
	if len(ids) != 1 {
		return "", fmt.Errorf("expected one container for %s, found %q", node, ids)
	}
	log, err := exec.CommandContext(ctx, "docker", "logs", "--timestamps", ids[0]).CombinedOutput()
	return string(log), err
}

func processRSS(pid int) (int64, error) {
	data, err := os.ReadFile(fmt.Sprintf("/proc/%d/status", pid))
	if err != nil {
		return 0, err
	}
	for _, line := range strings.Split(string(data), "\n") {
		if f := strings.Fields(line); len(f) >= 2 && f[0] == "VmRSS:" {
			return strconv.ParseInt(f[1], 10, 64)
		}
	}
	return 0, fmt.Errorf("no VmRSS for %d", pid)
}

func meminfo(t *testing.T, key string) int64 {
	t.Helper()
	data, err := os.ReadFile("/proc/meminfo")
	if err != nil {
		t.Fatal(err)
	}
	for _, line := range strings.Split(string(data), "\n") {
		if f := strings.Fields(line); len(f) >= 2 && f[0] == key+":" {
			v, _ := strconv.ParseInt(f[1], 10, 64)
			return v
		}
	}
	t.Fatalf("no %s in /proc/meminfo", key)
	return 0
}
