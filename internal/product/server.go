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
	Name           string   `json:"name"`
	Studio         bool     `json:"studio"`
	DisabledPages  []string `json:"disabledPages"`
	Preview        bool     `json:"preview"`
	PromptStore    bool     `json:"promptStore"`
	ExtraProtocols bool     `json:"extraProtocols"`
	BodyCapture    bool     `json:"bodyCapture"`
	Engine         string   `json:"engine"`
	Channels       []string `json:"channels"`
	ManagedBy      string   `json:"managedBy"`
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

// LoadConfig applies the distribution's listener and engine policy before
// validation and backend-dependent defaults are selected.
func LoadConfig(path string) (config.Config, error) {
	p := Server()
	cfg, err := config.LoadWithOptions(path, config.LoadOptions{DisablePreview: !p.Preview, ForceWAABackend: p.Engine})
	if err != nil {
		return config.Config{}, err
	}
	return Constrain(cfg), nil
}

// Constrain also applies to legacy .env imports and every runtime reload. No
// migration path can re-enable a browser or silently send a PG model via Build.
func Constrain(cfg config.Config) config.Config {
	p := Server()
	cfg.PreviewDisabled = !p.Preview
	cfg.WAABackend = p.Engine
	cfg.UpstreamChannels = slices.Clone(p.Channels)
	cfg.ModelChannels = ""
	cfg.BuildNativeNonstream = false
	cfg.RequestBodyLog = p.BodyCapture
	cfg.FakeStream = false
	cfg.StripThoughtSignatures = false
	cfg.FirstEventHedge = config.FirstEventHedgeOff
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
