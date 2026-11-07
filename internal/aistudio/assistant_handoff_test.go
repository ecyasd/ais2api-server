package aistudio

import (
	"encoding/json"
	"errors"
	"io"
	"strings"
	"testing"
	"time"
)

func assistantHandoffStep(number, status int64, text string, action any) []any {
	step := make([]any, 16)
	if text != "" {
		step[0] = []any{[]any{[]any{[]any{nil, text}}, "model"}}
	}
	step[9], step[11], step[12] = action, number, status
	return step
}

func assistantHandoffMessage(steps ...[]any) []any {
	return []any{nil, nil, steps, "fixture-applet"}
}

func assistantHandoffAction(tag, questions string) []any {
	action := make([]any, 22)
	action[21] = []any{"question", nil, "fixture action", []any{
		[]any{"tag", tag},
		[]any{"questions", questions},
	}}
	return action
}

func TestAssistantCompletedQuestionFinishesBeforeStreamEOF(t *testing.T) {
	call := "```function_calls\n" + `[{"id":"fixture-call","name":"lookup","arguments":{"x":1}}]` + "\n```"
	handoff := assistantHandoffAction(assistantQuestionTool, `[{"question":"`+assistantCallQuestion+`"}]`)
	for _, test := range []struct {
		name      string
		messages  []any
		wantCalls int
		wantText  string
	}{
		{
			name: "call_emitted_in_earlier_snapshot",
			messages: []any{
				assistantHandoffMessage(assistantHandoffStep(1, 2, call, nil)),
				assistantHandoffMessage(assistantHandoffStep(1, 1, call, handoff)),
			},
			wantCalls: 1,
		},
		{
			name: "call_and_handoff_complete_together",
			messages: []any{
				assistantHandoffMessage(assistantHandoffStep(1, 1, call, handoff)),
			},
			wantCalls: 1,
		},
		{
			name: "action_only_handoff_survives_later_step",
			messages: []any{
				assistantHandoffMessage(assistantHandoffStep(1, 2, call, nil)),
				assistantHandoffMessage(assistantHandoffStep(2, 1, "", handoff), assistantHandoffStep(3, 2, "", nil)),
			},
			wantCalls: 1,
		},
		{
			name: "ordinary_question",
			messages: []any{
				assistantHandoffMessage(assistantHandoffStep(1, 1, "", assistantHandoffAction(assistantQuestionTool, `[{"question":"Which value?","options":["one","two"]}]`))),
			},
			wantText: "Which value?\n- one\n- two",
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			var messages []string
			for _, message := range test.messages {
				raw, err := json.Marshal(message)
				if err != nil {
					t.Fatal(err)
				}
				messages = append(messages, string(raw))
			}
			reader, writer := io.Pipe()
			defer reader.Close()
			defer writer.Close()
			type decoded struct {
				events   []Event
				err      error
				reopened bool
			}
			result := make(chan decoded, 1)
			go func() {
				defer reader.Close()
				var received decoded
				decoder := &AssistantStreamDecoder{Functions: []string{"lookup"}, Reopen: func() (io.ReadCloser, error) {
					received.reopened = true
					return nil, errors.New("completed question must not reopen")
				}}
				received.err = DecodeAssistantStream(reader, decoder, func(event Event) error {
					received.events = append(received.events, event)
					return nil
				})
				result <- received
			}()
			written := make(chan error, 1)
			go func() {
				// Keep both arrays and the pipe open: no EOF or trailer is available.
				_, err := io.WriteString(writer, "[["+strings.Join(messages, ","))
				written <- err
			}()
			select {
			case received := <-result:
				if received.err != nil || received.reopened {
					t.Fatalf("completed handoff failed: err=%v reopened=%v", received.err, received.reopened)
				}
				var text strings.Builder
				calls, finishes := 0, 0
				for _, event := range received.events {
					switch event.Kind {
					case EventToolCall:
						calls++
						if event.FunctionCall == nil || event.FunctionCall.ID != "fixture-call" || event.FunctionCall.Name != "lookup" || string(event.FunctionCall.Arguments) != `{"x":1}` {
							t.Fatalf("tool call changed: %+v", event.FunctionCall)
						}
					case EventText:
						text.WriteString(event.Text)
					case EventFinish:
						finishes++
					}
				}
				if calls != test.wantCalls || finishes != 1 || text.String() != test.wantText {
					t.Fatalf("calls=%d finishes=%d text=%q", calls, finishes, text.String())
				}
			case <-time.After(2 * time.Second):
				t.Fatal("completed ask_question waited for the stream trailer or EOF")
			}
			if err := <-written; err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestAssistantUnfinishedOrUnrelatedActionDoesNotEndTurn(t *testing.T) {
	for _, test := range []struct {
		name   string
		status int64
		tag    string
	}{
		{name: "unfinished_question", status: 2, tag: assistantQuestionTool},
		{name: "unrelated_completed_action", status: 1, tag: "another_action"},
	} {
		t.Run(test.name, func(t *testing.T) {
			message := assistantHandoffMessage(assistantHandoffStep(1, test.status, "", assistantHandoffAction(test.tag, `[{"question":"fixture"}]`)))
			raw, err := json.Marshal([]any{[]any{message}})
			if err != nil {
				t.Fatal(err)
			}
			finished := false
			err = DecodeAssistantStream(strings.NewReader(string(raw)), &AssistantStreamDecoder{}, func(event Event) error {
				finished = finished || event.Kind == EventFinish
				return nil
			})
			if err == nil || finished {
				t.Fatalf("incomplete turn accepted: err=%v finished=%v", err, finished)
			}
		})
	}
}
