package app

import (
	"context"
	"errors"
	"io"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"github.com/Mag1cFall/AIStudio2API/internal/aistudio"
	"github.com/Mag1cFall/AIStudio2API/internal/requestdb"
	"github.com/Mag1cFall/AIStudio2API/internal/telemetry"
)

// timingEventService replaces only the upstream transport; attempts, leases,
// unary collection, settlement, forwarding and the timing ledger remain real.
type timingEventService func(context.Context, aistudio.GenerateRequest) (<-chan aistudio.Event, error)

func (service timingEventService) Models(context.Context) ([]aistudio.Model, error) {
	return nil, nil
}

func (service timingEventService) CountTokens(context.Context, aistudio.TokenCountRequest) (aistudio.TokenCount, error) {
	return aistudio.TokenCount{}, nil
}

func (service timingEventService) Generate(ctx context.Context, request aistudio.GenerateRequest) (<-chan aistudio.Event, error) {
	return service(ctx, request)
}

func TestUnaryInterruptedAttemptTimingSurvivesRetryAndRestart(t *testing.T) {
	for _, firstKind := range []aistudio.EventKind{"", aistudio.EventReasoning, aistudio.EventText} {
		name := "before_first_event"
		if firstKind != "" {
			name = "after_" + string(firstKind)
		}
		t.Run(name, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
			defer cancel()
			ctx = telemetry.WithTrace(ctx, time.Now())
			trace := telemetry.From(ctx)
			path := filepath.Join(t.TempDir(), "requests.db")
			log := func(level, message string) { t.Errorf("timing ledger %s: %s", level, message) }
			store, err := requestdb.Open(path, log)
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() {
				if err := store.Close(); err != nil {
					t.Error(err)
				}
			})
			request := aistudio.GenerateRequest{ID: "synthetic-retry", Model: "fixture-model", Unary: true}
			trace.Publish(func(report telemetry.Report) { store.RecordTiming(request.ID, report) })
			accounts := []*aistudio.Account{
				{ID: "interrupted@example.com", Config: aistudio.DefaultAccountConfig("Interrupted fixture"), State: aistudio.AccountReady},
				{ID: "completed@example.com", Config: aistudio.DefaultAccountConfig("Completed fixture"), State: aistudio.AccountReady},
			}
			interrupted := &aistudio.StreamInterruptedError{Method: "fixture", Frames: 1, Err: io.ErrUnexpectedEOF}
			upstream := timingEventService(func(_ context.Context, request aistudio.GenerateRequest) (<-chan aistudio.Event, error) {
				events := make(chan aistudio.Event, 2)
				if request.AccountID == accounts[0].ID {
					if firstKind != "" {
						events <- aistudio.Event{Kind: firstKind, Text: "discarded synthetic output"}
					}
					events <- aistudio.Event{Kind: aistudio.EventError, Err: interrupted}
				} else {
					events <- aistudio.Event{Kind: aistudio.EventText, Text: "completed synthetic output"}
					events <- aistudio.Event{Kind: aistudio.EventFinish, FinishReason: "stop"}
				}
				close(events)
				return events, nil
			})
			service := &trackedService{
				service: upstream, lifecycle: ctx, pool: aistudio.NewAccountPool(accounts, 1),
				workers: &accountWorkerManager{}, requests: newRequestRegistry(ctx),
			}
			service.requests.start(request, cancel)
			start := func(accountID string) *generateAttempt {
				t.Helper()
				lease, err := service.pool.AcquireAccount(ctx, accountID)
				if err != nil {
					t.Fatal(err)
				}
				t.Cleanup(func() { _ = lease.Release() })
				attempt := service.startAttempt(ctx, request, "", request.Model, "", lease, aistudio.AccountSelection{}, false)
				t.Cleanup(attempt.cancel)
				select {
				case <-attempt.done:
					return attempt
				case <-ctx.Done():
					t.Fatal("attempt did not finish waiting for its first event", ctx.Err())
					return nil
				}
			}

			failed := start(accounts[0].ID)
			beforeSettlement := trace.Snapshot().Attempts[0]
			if firstKind == "" {
				if !errors.Is(failed.err, interrupted) || beforeSettlement.State != "failed" {
					t.Fatal("failure before the first event was not recorded", failed.err, beforeSettlement)
				}
			} else {
				if failed.err != nil || failed.first.Kind != firstKind || beforeSettlement.State != "running" {
					t.Fatal("attempt did not reach its first event", failed.err, beforeSettlement)
				}
				_, collectErr := collectGenerateEvents(ctx, failed.source, func(time.Duration, []aistudio.Event) {})
				if !errors.Is(collectErr, interrupted) {
					t.Fatal("unary collection did not report the interrupted stream", collectErr)
				}
				failed.err = collectErr
			}
			retry := &generateRetry{maxAttempts: 2, attempted: map[string]struct{}{accounts[0].ID: {}}, recoveredWorker: make(map[string]struct{})}
			outcome := service.settleAttempt(ctx, request.ID, request.Model, failed, retry, false)
			if !outcome.retryable || !errors.Is(outcome.err, interrupted) {
				t.Fatal("interrupted attempt was not eligible for retry", outcome)
			}
			settled := trace.Snapshot().Attempts[0]
			if settled.State != "failed" || settled.DurationMS <= 0 {
				t.Fatalf("settled attempt retained an unfinished timing: %+v", settled)
			}
			if firstKind == "" && settled != beforeSettlement {
				t.Fatalf("settlement changed an already completed timing: before=%+v after=%+v", beforeSettlement, settled)
			}

			winner := start(accounts[1].ID)
			if winner.err != nil {
				t.Fatal(winner.err)
			}
			collected, err := collectGenerateEvents(ctx, winner.source, func(time.Duration, []aistudio.Event) {})
			if err != nil {
				t.Fatal(err)
			}
			destination := make(chan aistudio.Event, 2)
			forwardErr := service.forwardEvents(ctx, ctx, winner.cancel, request.ID,
				winner.first, collected, destination, winner.lease, winner.copies, winner.activity, request.Model)
			winner.timingEnd(forwardErr == nil)
			if forwardErr != nil {
				t.Fatal(forwardErr)
			}
			var kinds []aistudio.EventKind
			for event := range destination {
				kinds = append(kinds, event.Kind)
			}
			if !reflect.DeepEqual(kinds, []aistudio.EventKind{aistudio.EventText, aistudio.EventFinish}) {
				t.Fatal("retry did not forward a complete response", kinds)
			}
			report := trace.Snapshot()
			if len(report.Attempts) != 2 || report.Attempts[1].State != "completed" || report.Attempts[1].DurationMS <= 0 {
				t.Fatalf("retry timings were not completed: %+v", report.Attempts)
			}
			if err := store.Close(); err != nil {
				t.Fatal(err)
			}
			reopened, err := requestdb.Open(path, log)
			if err != nil {
				t.Fatal(err)
			}
			defer reopened.Close()
			persisted, err := reopened.RequestTiming(ctx, request.ID)
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(persisted.Attempts, report.Attempts) {
				t.Fatalf("retry timings changed after restart: got=%+v want=%+v", persisted.Attempts, report.Attempts)
			}
		})
	}
}
