package api

import (
	"context"
	"github.com/Mag1cFall/AIStudio2API/internal/telemetry"
	"net/http"
	"strings"
)

func (s *server) publicAuthentication(next http.Handler) http.Handler {
	if s.config.Product == nil {
		return browserOriginMiddleware(s.config.APIKey, authMiddleware(s.config.APIKey, apiKeyContextMiddleware(s.config.APIKey, next)))
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		keys := s.config.APIKey
		if s.config.CurrentAPIKeys != nil {
			keys = s.config.CurrentAPIKeys()
		}
		if len(apiKeys(keys)) == 0 || !checkAPIKeyMatches(r, apiKeys(keys)) {
			writeAuthError(w, r)
			return
		}
		markAccessLogAuthorized(r.Context())
		browserOriginMiddleware(keys, apiKeyContextMiddleware(keys, next)).ServeHTTP(w, r)
	})
}

type timingLedger interface {
	RequestTiming(context.Context, string) (telemetry.Report, error)
}

func (s *server) handleRequestTiming(w http.ResponseWriter, r *http.Request) {
	ledger, ok := s.config.Ledger.(timingLedger)
	if !ok {
		writeRouteNotFound(w, r)
		return
	}
	report, err := ledger.RequestTiming(r.Context(), r.PathValue("id"))
	if err != nil {
		writeAdminError(w, http.StatusNotFound, "timing_not_found", "此请求没有保留阶段耗时")
		return
	}
	writeJSON(w, http.StatusOK, report)
}

func (s *server) serverGeminiAction(w http.ResponseWriter, r *http.Request) {
	if s.config.Product != nil && !s.config.Product.ExtraProtocols {
		_, action, _ := strings.Cut(r.PathValue("action"), ":")
		switch action {
		case "generateContent", "streamGenerateContent", "countTokens":
		default:
			writeRouteNotFound(w, r)
			return
		}
	}
	s.handleGeminiAction(w, r)
}
