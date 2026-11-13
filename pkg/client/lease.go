package client

import (
	"context"
	"fmt"
	"sync"
	"time"

	labv1 "github.com/appmana/labcontainers/api/v1"
	"github.com/appmana/labcontainers/pkg/checkpoint"
)

// Extend moves the session's lease to at least ttl from now. Unlike Keep it
// leaves cleanup with the owner: closing the client still destroys the lab.
func (s *Session) Extend(ctx context.Context, ttl time.Duration) error {
	if ttl < time.Second {
		return fmt.Errorf("lease extension must be at least one second")
	}
	p, err := s.client.rpc.ExtendSession(ctx, &labv1.ExtendSessionRequest{Id: s.ID(), TtlSeconds: int64(ttl / time.Second)})
	if err != nil {
		return err
	}
	s.value = p
	return nil
}

// Expires is the lease's end as last reported by the daemon.
func (s *Session) Expires() time.Time { return time.Unix(s.value.GetExpiresUnix(), 0) }

// Lease renews a session for as long as its holder runs.
type Lease struct {
	stop func()
	done chan struct{}
	mu   sync.Mutex
	err  error
}

// HoldLease renews the session every third of ttl until the returned lease is
// released or ctx ends. A run of any length then keeps its lab, while a holder
// that dies stops renewing and the daemon reclaims the lab one ttl later.
// The first renewal happens before HoldLease returns.
func (s *Session) HoldLease(ctx context.Context, ttl time.Duration) (*Lease, error) {
	if ttl < 3*time.Second {
		return nil, fmt.Errorf("a held lease needs a ttl of at least three seconds")
	}
	return holdLease(ctx, ttl/3, func(ctx context.Context) error {
		_, err := s.client.rpc.ExtendSession(ctx, &labv1.ExtendSessionRequest{Id: s.ID(), TtlSeconds: int64(ttl / time.Second)})
		return err
	})
}

func holdLease(ctx context.Context, every time.Duration, renew func(context.Context) error) (*Lease, error) {
	if err := renew(ctx); err != nil {
		return nil, err
	}
	ctx, cancel := context.WithCancel(ctx)
	lease := &Lease{stop: cancel, done: make(chan struct{})}
	go func() {
		defer close(lease.done)
		ticker := time.NewTicker(every)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
			}
			// One failed renewal is not fatal: two more follow before the
			// lease runs out. Report the latest outcome to the holder.
			err := renew(ctx)
			if ctx.Err() != nil {
				return
			}
			lease.mu.Lock()
			lease.err = err
			lease.mu.Unlock()
		}
	}()
	return lease, nil
}

// Err is the outcome of the most recent renewal.
func (l *Lease) Err() error {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.err
}

// Release stops renewing. The lease then runs out one ttl after its last
// renewal unless the session is destroyed or kept first.
func (l *Lease) Release() {
	l.stop()
	<-l.done
}

// Checkpoint saves the disks of stopped nodes into dir, which must be empty
// or absent. With no nodes named, every VM node is saved and must be stopped.
// The directory is outside session state and survives the session.
func (s *Session) Checkpoint(ctx context.Context, dir string, labels map[string]string, nodes ...string) (*labv1.Checkpoint, error) {
	return s.client.rpc.CreateCheckpoint(ctx, &labv1.CheckpointRequest{SessionId: s.ID(), Directory: dir, Nodes: nodes, Labels: labels})
}

// Restore replaces the disks of stopped nodes with a checkpoint's content.
// With no nodes named, every node in the checkpoint is restored. The session
// may be the one that made the checkpoint or a new one with the same nodes,
// images and declared disks. Start the nodes afterwards.
func (s *Session) Restore(ctx context.Context, dir string, nodes ...string) (*labv1.Checkpoint, error) {
	return s.client.rpc.RestoreCheckpoint(ctx, &labv1.CheckpointRequest{SessionId: s.ID(), Directory: dir, Nodes: nodes})
}

// ReadCheckpoint inspects a checkpoint directory without a daemon.
// checkpoint.ErrNone means dir holds no complete checkpoint.
func ReadCheckpoint(dir string) (checkpoint.Manifest, error) { return checkpoint.Read(dir) }
