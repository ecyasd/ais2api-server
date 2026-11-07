package app

import (
	"os"
	"testing"
	"time"
)

func TestWorkerIdleTimeoutConfiguration(t *testing.T) {
	original, existed := os.LookupEnv("WORKER_IDLE_TIMEOUT")
	t.Cleanup(func() {
		if existed {
			_ = os.Setenv("WORKER_IDLE_TIMEOUT", original)
		} else {
			_ = os.Unsetenv("WORKER_IDLE_TIMEOUT")
		}
	})
	_ = os.Unsetenv("WORKER_IDLE_TIMEOUT")
	if got := configuredWorkerIdleTimeout(); got != 30*time.Minute {
		t.Fatalf("unset retention = %s, want 30m", got)
	}
	for _, test := range []struct {
		value string
		want  time.Duration
	}{
		{"30m", 30 * time.Minute},
		{"10m", 10 * time.Minute},
		{" 1h ", time.Hour},
		{"1s", time.Second},
		{"", 30 * time.Minute},
		{"0", 30 * time.Minute},
		{"-1m", 30 * time.Minute},
		{"500ms", 30 * time.Minute},
		{"invalid", 30 * time.Minute},
	} {
		t.Run(test.value, func(t *testing.T) {
			t.Setenv("WORKER_IDLE_TIMEOUT", test.value)
			manager := &accountWorkerManager{idleAfter: configuredWorkerIdleTimeout()}
			if got := manager.idleTimeout(); got != test.want {
				t.Fatalf("retention = %s, want %s", got, test.want)
			}
		})
	}
}

func TestWorkerIdleRetentionBoundaries(t *testing.T) {
	manager := &accountWorkerManager{idleAfter: 30 * time.Minute}
	now := time.Date(2026, 1, 1, 1, 0, 0, 0, time.UTC)
	cutoff := now.Add(-manager.idleTimeout())
	if !now.Add(-29*time.Minute - 59*time.Second).After(cutoff) {
		t.Fatal("a worker idle less than 30 minutes became eligible")
	}
	if now.Add(-30 * time.Minute).After(cutoff) {
		t.Fatal("a worker idle exactly 30 minutes remained recent")
	}
	if got := workerIdleCheckInterval(manager.idleTimeout()); got != 30*time.Second {
		t.Fatalf("poll interval = %s, want 30s", got)
	}
	if got := workerIdleCheckInterval(time.Second); got != time.Second {
		t.Fatalf("short timeout poll interval = %s", got)
	}
}

func TestIdleReaperKeepsWarmFloor(t *testing.T) {
	account := &accountWorker{id: "synthetic-warm-account"}
	account.warm.Store(true)
	manager := &accountWorkerManager{
		accounts:  map[string]*accountWorker{account.id: account},
		openings:  make(map[string]chan struct{}),
		idleAfter: 30 * time.Minute,
	}
	manager.warmTarget.Store(1)
	manager.maxActive.Store(4)
	// Even a very old idle cutoff must not evict the final warm worker.
	if manager.reapIdleWorker(time.Now().Add(time.Hour)) {
		t.Fatal("the last warm worker was reaped")
	}
	if !account.warm.Load() {
		t.Fatal("the last warm worker was marked cold")
	}
}
