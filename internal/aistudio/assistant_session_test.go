package aistudio

import (
	"encoding/json"
	"testing"
)

func assistantResultHistory(firstCall, secondCall, firstResult, secondResult string, named bool) []Content {
	resultName := ""
	if named {
		resultName = "lookup"
	}
	return []Content{
		{Role: RoleUser, Parts: []Part{{Text: "Look up both values."}}},
		{Role: RoleAssistant, Parts: []Part{
			{FunctionCall: &FunctionCall{ID: firstCall, Name: "lookup", Arguments: json.RawMessage(`{"x":1}`)}},
			{FunctionCall: &FunctionCall{ID: secondCall, Name: "lookup", Arguments: json.RawMessage(`{"x":2}`)}},
		}},
		{Role: RoleTool, Parts: []Part{{FunctionResult: &FunctionResult{ID: firstResult, Name: resultName, Content: json.RawMessage(`"red"`)}}}},
		{Role: RoleTool, Parts: []Part{{FunctionResult: &FunctionResult{ID: secondResult, Name: resultName, Content: json.RawMessage(`"blue"`)}}}},
		{Role: RoleAssistant, Parts: []Part{{Text: "I have both values."}}},
		{Role: RoleUser, Parts: []Part{{Text: "Which value belongs to x=1?"}}},
	}
}

func TestAssistantSessionReusePreservesToolResultAssociations(t *testing.T) {
	original := assistantResultHistory("a", "b", "a", "b", false)
	links := assistantLinks(original[:5])
	for _, test := range []struct {
		name    string
		history []Content
		reuse   bool
	}{
		{name: "same", history: original, reuse: true},
		{name: "consistent_ID_rename", history: assistantResultHistory("renamed-a", "renamed-b", "renamed-a", "renamed-b", false), reuse: true},
		{name: "optional_result_name", history: assistantResultHistory("a", "b", "a", "b", true), reuse: true},
		{name: "swapped_result_associations", history: assistantResultHistory("a", "b", "b", "a", false)},
		{name: "unknown_result_ID", history: assistantResultHistory("a", "b", "missing", "b", false)},
	} {
		t.Run(test.name, func(t *testing.T) {
			store := newAssistantSessions()
			session := store.open("fixture-account", "fixture-applet")
			store.finish(session, &assistantSessionKey{apiKey: "fixture-key", digest: links[len(links)-1].digest}, false)
			claimed, start := store.claim("fixture-key", "fixture-account", assistantLinks(test.history))
			if test.reuse {
				if claimed != session || start != 5 {
					t.Fatalf("equivalent history was not reused: session=%p start=%d", claimed, start)
				}
			} else if claimed != nil || start != 0 {
				t.Fatalf("changed association reused an existing applet: session=%p start=%d", claimed, start)
			}
		})
	}
}

func TestPlaygroundHistoryRestorationPreservesToolResultAssociations(t *testing.T) {
	original := assistantResultHistory("a", "b", "a", "b", false)
	for _, test := range []struct {
		name    string
		history []Content
		restore bool
	}{
		{name: "consistent_ID_rename", history: assistantResultHistory("renamed-a", "renamed-b", "renamed-a", "renamed-b", false), restore: true},
		{name: "swapped_result_associations", history: assistantResultHistory("a", "b", "b", "a", false)},
	} {
		t.Run(test.name, func(t *testing.T) {
			turns := newModelTurnHistory()
			turns.store("fixture-key", original[:4], modelTurn{parts: []Part{
				{Text: "synthetic private thought", Thought: true},
				{Text: "I have both values.", ThoughtSignature: "synthetic-signature"},
			}, turnToken: "synthetic-token"})
			restored, token := turns.restore("fixture-key", test.history)
			thoughts, signatures := 0, 0
			for _, content := range restored {
				for _, part := range content.Parts {
					if part.Thought {
						thoughts++
					}
					if part.ThoughtSignature == "synthetic-signature" {
						signatures++
					}
				}
			}
			if test.restore {
				if token != "synthetic-token" || thoughts != 1 || signatures != 1 {
					t.Fatalf("equivalent history lost PG state: token=%q thoughts=%d signatures=%d", token, thoughts, signatures)
				}
			} else if token != "" || thoughts != 0 || signatures != 0 {
				t.Fatalf("changed association restored unrelated PG state: token=%q thoughts=%d signatures=%d", token, thoughts, signatures)
			}
		})
	}
}
