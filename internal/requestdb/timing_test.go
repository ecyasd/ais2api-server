package requestdb

import (
	"context"
	"github.com/Mag1cFall/AIStudio2API/internal/telemetry"
	"path/filepath"
	"testing"
	"time"
)

func TestTimingRestartAndOutOfOrderCleanup(t *testing.T) {
	path := filepath.Join(t.TempDir(), "requests.db")
	log := func(level, message string) { t.Errorf("ledger failure: %s", level) }
	store, err := Open(path, log)
	if err != nil {
		t.Fatal(err)
	}
	value := 1234.0
	store.RecordTiming("request", telemetry.Report{StartedAt: time.Now(), Revision: 2, FirstTool: &value, BackgroundCleanup: &value, Output: "tools",
		Failure: &telemetry.Failure{Kind: "upstream_rpc", Method: "GenerateContent", HTTPStatus: 400, RPCCode: 3, Reason: "invalid_argument"}})
	store.RecordTiming("request", telemetry.Report{StartedAt: time.Now(), Revision: 1, Output: "none"})
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	store, err = Open(path, log)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	report, err := store.RequestTiming(context.Background(), "request")
	if err != nil || report.Revision != 2 || report.BackgroundCleanup == nil || report.FirstBody != nil {
		t.Fatal("newer terminal timing lost after restart", err)
	}
	if report.Failure == nil || report.Failure.Reason != "invalid_argument" || report.Failure.RPCCode != 3 {
		t.Fatal("safe failure diagnosis lost after restart")
	}
}
