package client

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"
)

func TestHeldLeaseRenewsUntilReleased(t *testing.T) {
	var renewals atomic.Int32
	lease, err := holdLease(context.Background(), 5*time.Millisecond, func(context.Context) error {
		renewals.Add(1)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if renewals.Load() != 1 {
		t.Fatal("the first renewal must happen before the lease is handed out")
	}
	deadline := time.Now().Add(2 * time.Second)
	for renewals.Load() < 4 && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	lease.Release()
	after := renewals.Load()
	if after < 4 {
		t.Fatalf("renewed %d times", after)
	}
	time.Sleep(25 * time.Millisecond)
	if renewals.Load() != after {
		t.Fatal("renewed after release")
	}
}

func TestHeldLeaseReportsFailedRenewals(t *testing.T) {
	boom := errors.New("daemon gone")
	if _, err := holdLease(context.Background(), time.Millisecond, func(context.Context) error { return boom }); !errors.Is(err, boom) {
		t.Fatalf("a lease that could not be taken was handed out: %v", err)
	}
	var calls atomic.Int32
	lease, err := holdLease(context.Background(), time.Millisecond, func(context.Context) error {
		if calls.Add(1) > 1 {
			return boom
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	defer lease.Release()
	deadline := time.Now().Add(2 * time.Second)
	for lease.Err() == nil && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	if !errors.Is(lease.Err(), boom) {
		t.Fatalf("lost the renewal failure: %v", lease.Err())
	}
}

func TestHeldLeaseStopsWithItsContext(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	lease, err := holdLease(ctx, time.Millisecond, func(context.Context) error { return nil })
	if err != nil {
		t.Fatal(err)
	}
	cancel()
	select {
	case <-lease.done:
	case <-time.After(2 * time.Second):
		t.Fatal("renewal outlived its context")
	}
	lease.Release()
}
