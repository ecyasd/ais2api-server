package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/Mag1cFall/AIStudio2API/internal/aistudio"
	"github.com/Mag1cFall/AIStudio2API/internal/product"
)

type serverFixtureService struct {
	mu       sync.Mutex
	requests []aistudio.GenerateRequest
}

func (*serverFixtureService) Models(context.Context) ([]aistudio.Model, error) {
	return []aistudio.Model{{ID: "fixture", Methods: []string{"generateContent"}, Capabilities: map[string]bool{"chat_model": true}}}, nil
}
func (*serverFixtureService) CountTokens(context.Context, aistudio.TokenCountRequest) (aistudio.TokenCount, error) {
	return aistudio.TokenCount{}, nil
}
func (service *serverFixtureService) Generate(ctx context.Context, request aistudio.GenerateRequest) (<-chan aistudio.Event, error) {
	service.mu.Lock()
	service.requests = append(service.requests, request)
	count := len(service.requests)
	service.mu.Unlock()
	events := make(chan aistudio.Event, 3)
	if count == 1 {
		events <- aistudio.Event{Kind: aistudio.EventToolCall, FunctionCall: &aistudio.FunctionCall{ID: "fixture-call", Name: "lookup", Arguments: json.RawMessage(`{"value":1}`), ThoughtSignature: "synthetic-signature"}}
	} else {
		events <- aistudio.Event{Kind: aistudio.EventText, Text: "continued"}
	}
	events <- aistudio.Event{Kind: aistudio.EventFinish, FinishReason: "stop"}
	close(events)
	return events, nil
}

func TestServerOpenAIToolRoundTripBothModelRoutes(t *testing.T) {
	for _, model := range []string{"fixture", "fixture-build"} {
		t.Run(model, func(t *testing.T) {
			service := &serverFixtureService{}
			profile := product.Server()
			handler := NewHandler(service, Config{Product: &profile, APIKey: "synthetic-call-key"})
			post := func(body string) *httptest.ResponseRecorder {
				r := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(body))
				r.Header.Set("Authorization", "Bearer synthetic-call-key")
				w := httptest.NewRecorder()
				handler.ServeHTTP(w, r)
				return w
			}
			first := post(`{"model":"` + model + `","messages":[{"role":"user","content":"synthetic"}],"tools":[{"type":"function","function":{"name":"lookup","parameters":{"type":"object"}}}]}`)
			if first.Code != 200 || !strings.Contains(first.Body.String(), "tool_calls") {
				t.Fatal("first turn failed", first.Code)
			}
			var response struct {
				Choices []struct {
					Message json.RawMessage `json:"message"`
				} `json:"choices"`
			}
			if err := json.Unmarshal(first.Body.Bytes(), &response); err != nil {
				t.Fatal(err)
			}
			// Emulate a client dropping the extra signature field; in-memory restoration
			// must still match ID, function and arguments on the second turn.
			var message map[string]any
			_ = json.Unmarshal(response.Choices[0].Message, &message)
			for _, item := range message["tool_calls"].([]any) {
				delete(item.(map[string]any), "extra_content")
			}
			assistant, _ := json.Marshal(message)
			second := post(`{"model":"` + model + `","messages":[{"role":"user","content":"synthetic"},` + string(assistant) + `,{"role":"tool","tool_call_id":"fixture-call","content":"synthetic-result"}]}`)
			if second.Code != 200 || !strings.Contains(second.Body.String(), "continued") {
				t.Fatal("tool result continuation failed", second.Code)
			}
			request := service.requests[1]
			if request.Model != model || request.Contents[1].Parts[0].FunctionCall.ThoughtSignature != "synthetic-signature" {
				t.Fatal("model route or signature lost")
			}
			invalid := post(`{"model":"` + model + `","messages":[` + string(assistant) + `,{"role":"tool","tool_call_id":"wrong-id","content":"result"}]}`)
			if invalid.Code != 400 {
				t.Fatal("mismatched result accepted", invalid.Code)
			}
		})
	}
}

func TestServerDynamicKeysAndExcludedRoutes(t *testing.T) {
	profile := product.Server()
	key := "one"
	handler := NewHandler(&serverFixtureService{}, Config{Product: &profile, CurrentAPIKeys: func() string { return key }})
	check := func(path, token string, want int) {
		r := httptest.NewRequest("GET", path, nil)
		r.Header.Set("Authorization", "Bearer "+token)
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, r)
		if w.Code != want {
			t.Fatalf("path=%s status=%d want=%d", path, w.Code, want)
		}
	}
	check("/v1/models", "one", 200)
	key = "two"
	check("/v1/models", "one", 401)
	check("/v1/models", "two", 200)
	check("/v1/live", "two", 404)
	key = ""
	check("/v1/models", "two", 401)
}
