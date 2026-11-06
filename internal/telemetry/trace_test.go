package telemetry

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"
)

func TestToolOnlyTimingAndLateCleanup(t *testing.T) {
	ctx := WithTrace(context.Background(), time.Now().Add(-time.Second))
	finish := AttemptStarted(ctx, "assistant")
	Semantic(ctx, "tool")
	Semantic(ctx, "finish")
	finish(true)
	trace := From(ctx)
	report := trace.Snapshot()
	if report.FirstBody != nil || report.FirstTool == nil || report.Output != "tools" || len(report.Attempts) != 1 {
		t.Fatal("tool-only timing fabricated a body")
	}
	updates := 0
	trace.Publish(func(report Report) { updates++ })
	CleanupDone(ctx, true)
	if updates != 2 || trace.Snapshot().BackgroundCleanup == nil {
		t.Fatal("late cleanup not published")
	}
}

func TestLogsNeverContainErrorPayloads(t *testing.T) {
	raw := []byte(`{"level":"ERROR","msg":"请求失败 | Cookie=fixture-secret","error":"prompt and signature fixture-secret","applet":"private-app"}`)
	redacted := JSONLog(raw)
	if strings.Contains(string(redacted), "fixture-secret") || !json.Valid(redacted) {
		t.Fatal("raw diagnostic payload leaked")
	}
}
