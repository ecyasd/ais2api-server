package api

import (
	"encoding/json"
	"fmt"
)

// validateChatToolHistory rejects broken associations before the upstream
// converter's legacy name-recovery heuristics can guess a different tool.
func validateChatToolHistory(messages []chatMessage) error {
	pending := map[string]string{}
	for _, message := range messages {
		for _, call := range message.ToolCalls {
			if call.Type != "function" {
				return fmt.Errorf("服务器版仅支持标准 function 工具")
			}
			if call.ID == "" || call.Function.Name == "" || pending[call.ID] != "" {
				return fmt.Errorf("工具调用 ID 缺失或重复")
			}
			var object map[string]json.RawMessage
			if json.Unmarshal([]byte(call.Function.Arguments), &object) != nil || object == nil {
				return fmt.Errorf("工具调用参数必须是完整 JSON object")
			}
			pending[call.ID] = call.Function.Name
		}
		if message.Role == "tool" {
			name, exists := pending[message.ToolCallID]
			if !exists || message.Name != "" && message.Name != name {
				return fmt.Errorf("工具结果没有匹配的调用 ID 或函数名")
			}
			delete(pending, message.ToolCallID)
		}
	}
	return nil
}
