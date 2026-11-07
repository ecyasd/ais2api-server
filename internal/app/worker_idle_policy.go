package app

import (
	"log/slog"
	"os"
	"strings"
	"time"
)

// WORKER_IDLE_TIMEOUT is a process environment setting. It is read when
// a generation service creates its worker manager. It does not remove
// credentials or lower WARM_WORKER_LIMIT; only surplus idle workers expire.
func configuredWorkerIdleTimeout() time.Duration {
	value, present := os.LookupEnv("WORKER_IDLE_TIMEOUT")
	if !present {
		return workerIdleTimeout
	}
	duration, err := time.ParseDuration(strings.TrimSpace(value))
	if err != nil || duration < time.Second {
		// Do not echo environment contents into logs.
		slog.Warn("WORKER_IDLE_TIMEOUT must be a duration of at least 1s; using 30m")
		return workerIdleTimeout
	}
	return duration
}

func (manager *accountWorkerManager) idleTimeout() time.Duration {
	if manager.idleAfter < time.Second {
		return workerIdleTimeout
	}
	return manager.idleAfter
}

// A long retention period must not also delay capacity-shrink checks.
func workerIdleCheckInterval(retention time.Duration) time.Duration {
	return min(30*time.Second, max(time.Second, retention/10))
}
