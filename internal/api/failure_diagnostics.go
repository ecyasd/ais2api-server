package api

import (
	"context"
	"errors"
	"net"
	"net/http"
	"strings"

	"github.com/Mag1cFall/AIStudio2API/internal/aistudio"
	"github.com/Mag1cFall/AIStudio2API/internal/telemetry"
)

// failureDiagnostic projects the typed error before logging discards its text.
// Only fixed labels, a known operation name and numeric codes may leave here.
func failureDiagnostic(err error) telemetry.Failure {
	failure := telemetry.Failure{Kind: "request", HTTPStatus: statusFromError(err), Reason: "request_failed"}
	switch {
	case errors.Is(err, context.Canceled):
		failure.Reason, failure.HTTPStatus = "canceled", 499
		return failure
	case errors.Is(err, context.DeadlineExceeded):
		failure.Reason = "timeout"
		return failure
	}
	var rpc *aistudio.RPCError
	if errors.As(err, &rpc) {
		failure.Kind = "upstream_rpc"
		failure.HTTPStatus = validHTTPStatus(rpc.StatusCode)
		if rpc.Code >= 0 && rpc.Code <= 16 {
			failure.RPCCode = rpc.Code
		}
		switch rpc.Method {
		case "GenerateContent", "CodeAssistantOffline", "ProxyStreamedCall", "ProxyUnaryCall":
			failure.Method = rpc.Method
		default:
			failure.Method = "other"
		}
		message := strings.ToLower(rpc.Message)
		switch {
		case strings.Contains(message, "thought_signature"), strings.Contains(message, "thought signature"):
			failure.Reason = "thought_signature"
		case rpc.ContextOverflow():
			failure.Reason = "context_length"
		case rpc.Code == 9:
			failure.Reason = "failed_precondition"
		case rpc.Code == 3 || rpc.StatusCode == http.StatusBadRequest:
			failure.Reason = "invalid_argument"
		case rpc.Code == 16 || rpc.StatusCode == http.StatusUnauthorized:
			failure.Reason = "authentication"
		case rpc.Code == 7 || rpc.StatusCode == http.StatusForbidden:
			failure.Reason = "permission_denied"
		case rpc.Code == 8 || rpc.StatusCode == http.StatusTooManyRequests:
			failure.Reason = "quota_exceeded"
		case rpc.Code == 4 || rpc.StatusCode == http.StatusGatewayTimeout:
			failure.Reason = "timeout"
		case rpc.Code == 14 || rpc.StatusCode == http.StatusServiceUnavailable:
			failure.Reason = "unavailable"
		default:
			failure.Reason = "upstream_error"
		}
		return failure
	}
	var network net.Error
	switch {
	case isUnverifiedProtocolError(err):
		failure.Reason = "protocol_mismatch"
	case errors.As(err, &network):
		failure.Kind, failure.Reason = "transport", "network"
		if network.Timeout() {
			failure.Reason = "timeout"
		}
	}
	return failure
}
