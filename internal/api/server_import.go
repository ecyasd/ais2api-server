package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/Mag1cFall/AIStudio2API/internal/aistudio"
)

// serverImportInput accepts full native state, extension JSON, Netscape text or
// a Cookie header. Only native state claims to preserve all original attributes.
type serverImportInput struct {
	Data     string `json:"data"`
	Label    string `json:"label"`
	Locale   string `json:"locale"`
	Timezone string `json:"timezone"`
	AuthUser int    `json:"auth_user"`
}

func parseServerImport(input serverImportInput) (AccountStateInput, bool, error) {
	text := strings.TrimSpace(strings.TrimPrefix(input.Data, "\ufeff"))
	var state aistudio.StorageState
	var envelope AccountStateInput
	var object map[string]json.RawMessage
	complete := false
	if json.Unmarshal([]byte(text), &object) == nil && object != nil {
		if err := json.Unmarshal([]byte(text), &envelope); err != nil {
			return envelope, false, fmt.Errorf("完整账户状态格式无效")
		}
		_, wrapped := object["storage_state"]
		if raw, exists := object["storage_state"]; exists {
			text = string(raw)
		}
		native := wrapped || object["origins"] != nil || object["aistudio2api"] != nil
		if err := json.Unmarshal([]byte(text), &state); native && err == nil && len(state.Cookies) > 0 && state.Validate() == nil {
			complete = true
		}
	}
	if !complete {
		var err error
		state, err = aistudio.ParseCookieText(text)
		if err != nil {
			return envelope, false, fmt.Errorf("Cookie 格式或必要登录字段无效")
		}
	}
	if _, err := aistudio.NewSigner().Authorization(state); err != nil {
		return envelope, false, fmt.Errorf("缺少有效的签名 Cookie；请重新导出完整状态")
	}
	envelope.StorageState = state
	envelope.ImportCompleteness = "partial"
	if complete {
		envelope.ImportCompleteness = "full_state"
	}
	if input.Label != "" {
		envelope.Label = input.Label
	}
	if input.Locale != "" {
		envelope.Locale = input.Locale
	}
	if input.Timezone != "" {
		envelope.Timezone = input.Timezone
	}
	if input.AuthUser != 0 {
		envelope.AuthUser = input.AuthUser
	}
	return envelope, complete, nil
}

func (s *server) handleServerImport(w http.ResponseWriter, r *http.Request) {
	var input serverImportInput
	if err := decodeJSON(r, &input); err != nil {
		writeAdminError(w, http.StatusBadRequest, "invalid_import", "导入内容格式无效")
		return
	}
	state, complete, err := parseServerImport(input)
	if err != nil {
		writeAdminError(w, http.StatusBadRequest, "invalid_import", err.Error())
		return
	}
	account, err := s.config.Admin.ImportAccountState(r.Context(), state)
	if err != nil {
		writeAdminUpstreamError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"account": account, "complete_state": complete, "verified_online": false})
}
