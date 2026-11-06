package telemetry

import (
	"encoding/json"
	"strings"
)

// Message keeps fixed operation names, never the dynamic suffix (which can
// contain upstream payloads, signed URLs or errors with protocol evidence).
func Message(message string) string {
	name, _, _ := strings.Cut(message, " | ")
	name, _, _ = strings.Cut(name, "\n")
	switch name {
	case "请求开始", "请求完成", "工具调用完成", "请求失败", "请求已取消", "达到输出上限", "上游终止生成",
		"运行时装配", "协议运行时就绪", "管理监听启动", "管理服务就绪", "生成服务启动", "生成服务已停止",
		"请求准备等待", "请求准备结束", "上游首事件等待", "上游首事件到达", "等待上游响应", "排队等待", "排队结束",
		"上游调用", "事件流停顿", "事件流恢复", "账号切换", "账号冷却", "WAA Worker 重建", "WAA Worker 空闲回收",
		"服务配置已保存", "账户凭据已更新（仅保存）", "账户由配对电脑续签", "账户文件损坏，已隔离",
		"请求账本写入失败", "请求账本写入队列已满", "请求账本清理失败", "临时文件清理失败", "Build 写代码助手清理失败":
		return name
	default:
		return "运行状态更新（仅记录元数据）"
	}
}

// JSONLog also protects direct slog calls outside the management registry. Raw
// error, protocol evidence, applet IDs and arbitrary attributes are discarded.
func JSONLog(data []byte) []byte {
	var record map[string]json.RawMessage
	if json.Unmarshal(data, &record) != nil {
		return []byte("{\"msg\":\"runtime_log_redacted\"}\n")
	}
	var message string
	_ = json.Unmarshal(record["msg"], &message)
	safe := map[string]any{"msg": Message(message)}
	for _, key := range []string{"time", "level", "event", "source", "request"} {
		if value, ok := record[key]; ok {
			safe[key] = value
		}
	}
	encoded, _ := json.Marshal(safe)
	return append(encoded, '\n')
}
