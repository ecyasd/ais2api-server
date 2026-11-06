package aistudio

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestServerToolContractRejectsUndeclaredCallsAndInvalidStrictSchema(t *testing.T) {
	request := GenerateRequest{Tools: Tools{Functions: []FunctionDeclaration{{Name: "allowed", Parameters: json.RawMessage(`{"type":"object"}`)}}}}
	_, contract, err := prepareToolRequest(request)
	if err != nil || contract == nil {
		t.Fatal("auto mode must enforce declaration membership", err)
	}
	input := make(chan Event, 2)
	input <- Event{Kind: EventToolCall, FunctionCall: &FunctionCall{Name: "undeclared", Arguments: json.RawMessage(`{}`)}}
	close(input)
	if event := <-contract.forward(context.Background(), input); event.Kind != EventError {
		t.Fatal("undeclared call escaped")
	}
	request.Tools.Functions[0].Strict = true
	request.Tools.Functions[0].Parameters = json.RawMessage(`{"type":"object","properties":{"x":{"$ref":"https://external.invalid/x"}}}`)
	if _, _, err := prepareToolRequest(request); err == nil {
		t.Fatal("unresolvable strict schema was silently ignored")
	}
}

func TestServerToolModesAndSingleCallContract(t *testing.T) {
	for _, test := range []struct {
		name, mode string
		parallel   *bool
		events     []Event
	}{
		{"none", "none", nil, []Event{{Kind: EventToolCall, FunctionCall: &FunctionCall{Name: "f", Arguments: json.RawMessage(`{}`)}}}},
		{"required", "required", nil, []Event{{Kind: EventFinish}}},
		{"json", "auto", nil, []Event{{Kind: EventToolCall, FunctionCall: &FunctionCall{Name: "f", Arguments: json.RawMessage(`{"x":`)}}}},
		{"single", "auto", new(bool), []Event{{Kind: EventToolCall, FunctionCall: &FunctionCall{ID: "1", Name: "f", Arguments: json.RawMessage(`{}`)}}, {Kind: EventToolCall, FunctionCall: &FunctionCall{ID: "2", Name: "f", Arguments: json.RawMessage(`{}`)}}}},
		{"duplicate", "auto", nil, []Event{{Kind: EventToolCall, FunctionCall: &FunctionCall{ID: "1", Name: "f", Arguments: json.RawMessage(`{}`)}}, {Kind: EventToolCall, FunctionCall: &FunctionCall{ID: "1", Name: "f", Arguments: json.RawMessage(`{}`)}}}},
	} {
		t.Run(test.name, func(t *testing.T) {
			_, contract, err := prepareToolRequest(GenerateRequest{Tools: Tools{Functions: []FunctionDeclaration{{Name: "f"}}, ToolConfig: ToolConfig{Mode: test.mode, ParallelCalls: test.parallel}}})
			if err != nil {
				t.Fatal(err)
			}
			input := make(chan Event, len(test.events))
			for _, event := range test.events {
				input <- event
			}
			close(input)
			failed := false
			for event := range contract.forward(context.Background(), input) {
				failed = failed || event.Kind == EventError
			}
			if !failed {
				t.Fatal("invalid tool result accepted")
			}
		})
	}
}

func TestToolDeclarationsDoNotBufferOrdinaryBody(t *testing.T) {
	_, contract, err := prepareToolRequest(GenerateRequest{Tools: Tools{Functions: []FunctionDeclaration{{Name: "f"}}}})
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	input := make(chan Event)
	output := contract.forward(ctx, input)
	go func() {
		select {
		case input <- Event{Kind: EventText, Text: "first"}:
		case <-ctx.Done():
		}
	}()
	select {
	case event := <-output:
		if event.Text != "first" {
			t.Fatal("body changed")
		}
	case <-time.After(time.Second):
		t.Fatal("first body waited for request completion")
	}
	close(input)
}

func TestOldLeaseCannotOverwriteNewCredentialGeneration(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.json")
	state := StorageState{Cookies: []StateCookie{{Name: "SID", Value: "old", Domain: ".google.com", Path: "/", Expires: -1}}}
	if err := WriteStorageState(path, state); err != nil {
		t.Fatal(err)
	}
	account := &Account{ID: "fixture@example.com", StoragePath: path, StorageState: state}
	pool := NewAccountPool([]*Account{account}, 2)
	old := &AccountLease{pool: pool, account: account}
	fresh := &AccountLease{pool: pool, account: account}
	state.Cookies[0].Value = "new"
	if err := fresh.SaveStorageState(state); err != nil {
		t.Fatal(err)
	}
	if err := old.MergeSetCookieHeaders([]string{"SID=stale; Domain=.google.com; Path=/; Secure"}, "https://aistudio.google.com/", time.Now()); err != nil {
		t.Fatal(err)
	}
	reloaded, err := LoadStorageState(path)
	if err != nil || reloaded.Cookies[0].Value != "new" {
		t.Fatal("new credential overwritten", err)
	}
	if info, err := os.Stat(path); err != nil || info.Size() == 0 {
		t.Fatal("credential file lost", err)
	}
}

func TestCorruptAccountDoesNotRemoveHealthyAccount(t *testing.T) {
	root := t.TempDir()
	for _, name := range []string{"broken@example.com", "valid@example.com"} {
		dir := filepath.Join(root, name)
		if err := os.MkdirAll(dir, 0700); err != nil {
			t.Fatal(err)
		}
		cfg, _ := json.Marshal(DefaultAccountConfig(name))
		if err := os.WriteFile(filepath.Join(dir, accountConfigName), cfg, 0600); err != nil {
			t.Fatal(err)
		}
		if err := WriteStorageState(filepath.Join(dir, storageStateName), StorageState{Cookies: []StateCookie{{Name: "SID", Value: "fake", Domain: ".google.com", Path: "/", Expires: -1}}}); err != nil {
			t.Fatal(err)
		}
	}
	broken := filepath.Join(root, "broken@example.com", storageStateName)
	if err := os.WriteFile(broken, []byte("corrupt"), 0600); err != nil {
		t.Fatal(err)
	}
	store := NewAccountStore(root)
	isolated := ""
	store.OnLoadError = func(id string) { isolated = id }
	accounts, err := store.Load()
	if err != nil || len(accounts) != 1 || accounts[0].ID != "valid@example.com" || isolated != "broken@example.com" {
		t.Fatal("isolation failed", err)
	}
	if raw, _ := os.ReadFile(broken); string(raw) != "corrupt" {
		t.Fatal("isolation modified source file")
	}
}

func TestAssistantStreamsBodyAndPreservesToolResultIDs(t *testing.T) {
	decoder := &AssistantStreamDecoder{Functions: []string{"lookup"}}
	events := decoder.text("ordinary body")
	if len(events) != 1 || events[0].Kind != EventText || events[0].Text != "ordinary body" {
		t.Fatal("Build body buffered despite no tool call")
	}
	request := GenerateRequest{Contents: []Content{
		{Role: RoleAssistant, Parts: []Part{{FunctionCall: &FunctionCall{ID: "a", Name: "lookup", Arguments: json.RawMessage(`{"n":1}`)}}, {FunctionCall: &FunctionCall{ID: "b", Name: "lookup", Arguments: json.RawMessage(`{"n":2}`)}}}},
		{Role: RoleTool, Parts: []Part{{FunctionResult: &FunctionResult{ID: "b", Content: json.RawMessage(`{"result":2}`)}}, {FunctionResult: &FunctionResult{ID: "a", Content: json.RawMessage(`{"result":1}`)}}}},
	}}
	transcribed, err := assistantToolPrompt(request, 0, false)
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := json.Marshal(transcribed.Contents)
	if !strings.Contains(string(raw), `\"id\":\"b\"`) {
		t.Fatal("Build transcript lost call IDs")
	}
	level, err := assistantThinkingLevel(GenerationConfig{ReasoningEffort: "low"}, GenerationDefaults{ThinkingLevels: []int64{1, 2, 3}})
	if err != nil || level != 1 {
		t.Fatal("Build low reasoning effort changed", err)
	}
}
