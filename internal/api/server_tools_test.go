package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Mag1cFall/AIStudio2API/internal/product"
)

func TestServerRejectsCustomToolsBeforeFirstGeneration(t *testing.T) {
	for _, model := range []string{"fixture", "fixture-build"} {
		for _, test := range []struct {
			name   string
			fields string
		}{
			{name: "declaration", fields: `"tools":[{"type":"custom","custom":{"name":"lookup","format":{"type":"text"}}}]`},
			{name: "streaming_declaration", fields: `"stream":true,"tools":[{"type":"custom","custom":{"name":"lookup","format":{"type":"text"}}}]`},
			{name: "named_choice", fields: `"tools":[{"type":"function","function":{"name":"lookup"}}],"tool_choice":{"type":"custom","custom":{"name":"lookup"}}`},
			{name: "allowed_reference", fields: `"tools":[{"type":"function","function":{"name":"lookup"}}],"tool_choice":{"type":"allowed_tools","mode":"auto","tools":[{"type":"custom","name":"lookup"}]}`},
			{name: "nested_allowed_reference", fields: `"tools":[{"type":"function","function":{"name":"lookup"}}],"tool_choice":{"type":"allowed_tools","allowed_tools":{"mode":"auto","tools":[{"type":"custom","custom":{"name":"lookup"}}]}}`},
		} {
			t.Run(model+"/"+test.name, func(t *testing.T) {
				service := &serverFixtureService{}
				profile := product.Server()
				handler := NewHandler(service, Config{Product: &profile, APIKey: "synthetic-call-key"})
				body := `{"model":"` + model + `","messages":[{"role":"user","content":"synthetic"}],` + test.fields + `}`
				request := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(body))
				request.Header.Set("Authorization", "Bearer synthetic-call-key")
				response := httptest.NewRecorder()
				handler.ServeHTTP(response, request)
				var result struct {
					Error struct {
						Code    string `json:"code"`
						Message string `json:"message"`
					} `json:"error"`
				}
				if err := json.Unmarshal(response.Body.Bytes(), &result); err != nil {
					t.Fatalf("expected a JSON error before streaming: %v", err)
				}
				if response.Code != http.StatusBadRequest || result.Error.Code != "unsupported_tool_type" || !strings.Contains(result.Error.Message, "custom") {
					t.Fatalf("custom tool was not explicitly rejected: status=%d body=%s", response.Code, response.Body.String())
				}
				service.mu.Lock()
				defer service.mu.Unlock()
				if len(service.requests) != 0 {
					t.Fatal("custom tool reached the upstream generation service")
				}
			})
		}
	}
}
