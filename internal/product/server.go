// Package product owns the server distribution's product boundary. The same
// embedded document is imported by the Vue management application.
package product

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"slices"
	"strings"

	"github.com/Mag1cFall/AIStudio2API/internal/config"
)

// Profile is deliberately build-time policy, rather than a user-switchable mode.
type Profile struct {
	Name               string   `json:"name"`
	Studio             bool     `json:"studio"`
	DisabledPages      []string `json:"disabledPages"`
	BrowserLogin       bool     `json:"browserLogin"`
	Preview            bool     `json:"preview"`
	PromptStore        bool     `json:"promptStore"`
	ExtraProtocols     bool     `json:"extraProtocols"`
	BodyCapture        bool     `json:"bodyCapture"`
	Engine             string   `json:"engine"`
	Channels           []string `json:"channels"`
	WarmWorkers        int      `json:"warmWorkers"`
	ActiveWorkers      int      `json:"activeWorkers"`
	StartupConcurrency int      `json:"startupConcurrency"`
	ManagedBy          string   `json:"managedBy"`
}

//go:embed server.json
var document []byte

func Server() Profile {
	var profile Profile
	if err := json.Unmarshal(document, &profile); err != nil {
		panic(err)
	}
	return profile
}

// Constrain also applies to legacy .env imports and every runtime reload. No
// migration path can re-enable a browser or silently send a PG model via Build.
func Constrain(cfg config.Config) config.Config {
	p := Server()
	cfg.WAABackend = p.Engine
	cfg.UpstreamChannels = slices.Clone(p.Channels)
	cfg.ModelChannels = ""
	cfg.BuildNativeNonstream = false
	cfg.RequestBodyLog = p.BodyCapture
	cfg.FakeStream = false
	cfg.StripThoughtSignatures = false
	cfg.FirstEventHedge = config.FirstEventHedgeOff
	cfg.WarmWorkerLimit = min(max(1, cfg.WarmWorkerLimit), p.WarmWorkers)
	cfg.MaxActiveWorkers = min(max(cfg.WarmWorkerLimit, cfg.MaxActiveWorkers), p.ActiveWorkers)
	cfg.WarmStartupConcurrency = min(max(1, cfg.WarmStartupConcurrency), p.StartupConcurrency)
	cfg.AdminAuthEnabled = true
	return cfg
}

// ValidateCredentials prevents a management password from becoming a model key.
// An empty model key set is valid configuration, and denies all inference.
func ValidateCredentials(cfg config.Config) error {
	if strings.TrimSpace(cfg.AdminPassword) == "" || strings.TrimSpace(cfg.AdminUsername) == "" {
		return fmt.Errorf("服务器版需要独立管理用户名和密码")
	}
	if slices.Contains(config.APIKeys(cfg.ProxyAPIKey), cfg.AdminPassword) {
		return fmt.Errorf("管理密码与调用密钥必须不同")
	}
	return nil
}
