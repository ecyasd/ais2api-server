// Package telemetry records bounded timings and counts only. Its API has no
// field for prompt, response, thought, signature or credential content.
package telemetry

import (
	"context"
	"slices"
	"sync"
	"time"
)

type Stage string

const (
	Queue      Stage = "queue"
	Initialize Stage = "account_init"
	Signing    Stage = "waa_signing"
	Submit     Stage = "build_submit"
	Upstream   Stage = "upstream_headers"
)

type Attempt struct {
	Number     int     `json:"number"`
	Channel    string  `json:"channel"`
	DurationMS float64 `json:"duration_ms"`
	State      string  `json:"state"`
}

type Report struct {
	StartedAt         time.Time         `json:"started_at"`
	Stages            map[Stage]float64 `json:"stages_ms"`
	FirstThought      *float64          `json:"first_thought_ms"`
	FirstBody         *float64          `json:"first_body_ms"`
	FirstTool         *float64          `json:"first_tool_ms"`
	Generation        *float64          `json:"generation_ms"`
	Cleanup           *float64          `json:"cleanup_ms"`
	BackgroundCleanup *float64          `json:"background_cleanup_ms"`
	Output            string            `json:"output"`
	ThoughtEvents     int               `json:"thought_events"`
	BodyEvents        int               `json:"body_events"`
	ToolEvents        int               `json:"tool_events"`
	Attempts          []Attempt         `json:"attempts"`
	OmittedAttempts   int               `json:"omitted_attempts"`
}

type Trace struct {
	mu     sync.Mutex
	report Report
	sink   func(Report)
}

type traceKey struct{}

func WithTrace(ctx context.Context, started time.Time) context.Context {
	return context.WithValue(ctx, traceKey{}, &Trace{report: Report{StartedAt: started, Stages: map[Stage]float64{}, Output: "none"}})
}

func From(ctx context.Context) *Trace {
	trace, _ := ctx.Value(traceKey{}).(*Trace)
	return trace
}

func (trace *Trace) snapshotLocked() Report {
	report := trace.report
	report.Stages = make(map[Stage]float64, len(trace.report.Stages))
	for stage, value := range trace.report.Stages {
		report.Stages[stage] = value
	}
	report.Attempts = slices.Clone(report.Attempts)
	return report
}

func (trace *Trace) Snapshot() Report {
	if trace == nil {
		return Report{}
	}
	trace.mu.Lock()
	defer trace.mu.Unlock()
	return trace.snapshotLocked()
}

func Add(ctx context.Context, stage Stage, duration time.Duration) {
	if trace := From(ctx); trace != nil {
		trace.mu.Lock()
		trace.report.Stages[stage] += float64(duration) / float64(time.Millisecond)
		trace.mu.Unlock()
	}
}

func Start(ctx context.Context, stage Stage) func() {
	started := time.Now()
	return func() { Add(ctx, stage, time.Since(started)) }
}

func AttemptStarted(ctx context.Context, channel string) func(bool) {
	trace := From(ctx)
	if trace == nil {
		return func(bool) {}
	}
	started := time.Now()
	trace.mu.Lock()
	if len(trace.report.Attempts) >= 64 {
		trace.report.OmittedAttempts++
		trace.mu.Unlock()
		return func(bool) {}
	}
	index := len(trace.report.Attempts)
	trace.report.Attempts = append(trace.report.Attempts, Attempt{Number: index + 1, Channel: channel, State: "running"})
	trace.mu.Unlock()
	var once sync.Once
	return func(success bool) {
		once.Do(func() {
			trace.mu.Lock()
			attempt := &trace.report.Attempts[index]
			attempt.DurationMS = float64(time.Since(started)) / float64(time.Millisecond)
			attempt.State = "failed"
			if success {
				attempt.State = "completed"
			}
			sink, report := trace.sink, trace.snapshotLocked()
			trace.mu.Unlock()
			if sink != nil {
				sink(report)
			}
		})
	}
}

// Semantic is called after a non-empty event is emitted to the protocol client.
func Semantic(ctx context.Context, kind string) {
	trace := From(ctx)
	if trace == nil {
		return
	}
	trace.mu.Lock()
	defer trace.mu.Unlock()
	value := float64(time.Since(trace.report.StartedAt)) / float64(time.Millisecond)
	first := func(target **float64) {
		if *target == nil {
			*target = &value
		}
	}
	switch kind {
	case "thought":
		first(&trace.report.FirstThought)
		trace.report.ThoughtEvents++
	case "body":
		first(&trace.report.FirstBody)
		trace.report.BodyEvents++
	case "tool":
		first(&trace.report.FirstTool)
		trace.report.ToolEvents++
	case "finish":
		first(&trace.report.Generation)
	}
	switch {
	case trace.report.BodyEvents > 0 && trace.report.ToolEvents > 0:
		trace.report.Output = "text_and_tools"
	case trace.report.BodyEvents > 0:
		trace.report.Output = "text"
	case trace.report.ToolEvents > 0:
		trace.report.Output = "tools"
	case trace.report.ThoughtEvents > 0:
		trace.report.Output = "thought_only"
	}
}

func CleanupDone(ctx context.Context, background bool) {
	trace := From(ctx)
	if trace == nil {
		return
	}
	trace.mu.Lock()
	value := float64(time.Since(trace.report.StartedAt)) / float64(time.Millisecond)
	if background {
		trace.report.BackgroundCleanup = &value
	} else {
		trace.report.Cleanup = &value
	}
	sink, report := trace.sink, trace.snapshotLocked()
	trace.mu.Unlock()
	if sink != nil {
		sink(report)
	}
}

// Publish also captures a background cleanup that finished just before the sink
// was attached; the sink must never wait for remote cleanup or access content.
func (trace *Trace) Publish(sink func(Report)) {
	if trace == nil {
		return
	}
	trace.mu.Lock()
	trace.sink = sink
	report := trace.snapshotLocked()
	trace.mu.Unlock()
	sink(report)
}
