package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/Mag1cFall/AIStudio2API/internal/aistudio"
	"github.com/Mag1cFall/AIStudio2API/internal/telemetry"
)

// A generated 400 must leave a useful diagnosis without retaining its payload.
func TestAccessLogRetainsSafeUpstreamFailure(t *testing.T) {
	for _, test := range []struct {
		name, message, reason string
	}{
		{"signature", "Invalid thought signature: synthetic-private-value", "thought_signature"},
		{"context", "The input token count exceeds the maximum number of tokens allowed: synthetic-private-value", "context_length"},
		{"unknown", "unexpected upstream rejection: synthetic-private-value", "invalid_argument"},
	} {
		t.Run(test.name, func(t *testing.T) {
			ctx := telemetry.WithTrace(context.WithValue(context.Background(), accessLogContextKey{}, &accessLogMetadata{}), time.Now())
			err := fmt.Errorf("generation failed: %w", &aistudio.RPCError{
				Method: "GenerateContent", StatusCode: 400, Code: 3, Message: test.message,
				Metadata: map[string]string{"untrusted": "synthetic-private-value"},
			})
			SetAccessLogError(ctx, err)
			raw, err := json.Marshal(telemetry.From(ctx).Snapshot())
			if err != nil {
				t.Fatal(err)
			}
			var report struct {
				Failure struct {
					Kind       string `json:"kind"`
					Method     string `json:"method"`
					HTTPStatus int    `json:"http_status"`
					Code       int64  `json:"rpc_code"`
					Reason     string `json:"reason"`
				} `json:"failure"`
			}
			if err := json.Unmarshal(raw, &report); err != nil {
				t.Fatal(err)
			}
			failure := report.Failure
			if failure.Kind != "upstream_rpc" || failure.Method != "GenerateContent" || failure.HTTPStatus != 400 || failure.Code != 3 || failure.Reason != test.reason {
				t.Fatalf("upstream 400 lost its safe diagnosis: %+v", failure)
			}
			if strings.Contains(string(raw), "synthetic-private-value") {
				t.Fatal("upstream payload entered diagnostics")
			}
		})
	}
}

func TestFailureDiagnosticNeverCopiesUntrustedFields(t *testing.T) {
	failure := failureDiagnostic(&aistudio.RPCError{
		Method: "synthetic-private-value", StatusCode: 400, Code: 9,
		Message:  "unrecognized synthetic-private-value",
		Metadata: map[string]string{"reason": "synthetic-private-value"},
	})
	if failure.Method != "other" || failure.Reason != "failed_precondition" || failure.RPCCode != 9 {
		t.Fatalf("precondition failure was collapsed into a generic 400: %+v", failure)
	}
	raw, err := json.Marshal(failure)
	if err != nil || strings.Contains(string(raw), "synthetic-private-value") {
		t.Fatal("untrusted error fields entered diagnostics")
	}
	for _, test := range []struct {
		err    error
		reason string
		status int
	}{
		{context.Canceled, "canceled", 499},
		{context.DeadlineExceeded, "timeout", 504},
		{errors.New("synthetic-private-value"), "request_failed", 502},
	} {
		failure := failureDiagnostic(fmt.Errorf("synthetic-private-value: %w", test.err))
		if failure.Reason != test.reason || failure.HTTPStatus != test.status {
			t.Fatalf("local failure lost its fixed category: %+v", failure)
		}
	}
}
