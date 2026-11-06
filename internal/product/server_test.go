package product

import (
	"github.com/Mag1cFall/AIStudio2API/internal/config"
	"testing"
)

func TestLegacyConfigurationCannotEnableExcludedCapabilities(t *testing.T) {
	cfg := config.Default()
	cfg.WAABackend = config.WAABackendCamoufox
	cfg.UpstreamChannels = []string{"playground", "build"}
	cfg.ModelChannels = "gemini=build"
	cfg.RequestBodyLog, cfg.FakeStream, cfg.StripThoughtSignatures = true, true, true
	cfg = Constrain(cfg)
	if cfg.WAABackend != "spidermonkey" || len(cfg.UpstreamChannels) != 1 || cfg.UpstreamChannels[0] != "playground" || cfg.ModelChannels != "" || cfg.RequestBodyLog || cfg.FakeStream || cfg.StripThoughtSignatures || !cfg.AdminAuthEnabled {
		t.Fatal("legacy configuration escaped server boundary")
	}
	if cfg.MaxActiveWorkers != 2 || cfg.WarmWorkerLimit != 1 || cfg.WarmStartupConcurrency != 1 {
		t.Fatal("server resource baseline not applied")
	}
	cfg.AdminPassword = "synthetic-admin"
	cfg.ProxyAPIKey = ""
	if err := ValidateCredentials(cfg); err != nil {
		t.Fatal(err)
	}
	cfg.ProxyAPIKey = "other,synthetic-admin"
	if ValidateCredentials(cfg) == nil {
		t.Fatal("management password accepted as a model key")
	}
}
