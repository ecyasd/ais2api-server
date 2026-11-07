package product

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Mag1cFall/AIStudio2API/internal/config"
)

func TestLegacyConfigurationCannotEnableExcludedCapabilities(t *testing.T) {
	cfg := config.Default()
	cfg.WAABackend = config.WAABackendCamoufox
	cfg.UpstreamChannels = []string{"playground", "build"}
	cfg.ModelChannels = "gemini=build"
	cfg.RequestBodyLog, cfg.FakeStream, cfg.StripThoughtSignatures = true, true, true
	cfg.WarmWorkerLimit, cfg.MaxActiveWorkers, cfg.WarmStartupConcurrency = 3, 8, 6
	cfg = Constrain(cfg)
	if cfg.WAABackend != "spidermonkey" || len(cfg.UpstreamChannels) != 1 || cfg.UpstreamChannels[0] != "playground" || cfg.ModelChannels != "" || cfg.RequestBodyLog || cfg.FakeStream || cfg.StripThoughtSignatures || !cfg.AdminAuthEnabled {
		t.Fatal("legacy configuration escaped server boundary")
	}
	if cfg.MaxActiveWorkers != 8 || cfg.WarmWorkerLimit != 3 || cfg.WarmStartupConcurrency != 6 {
		t.Fatal("product boundary changed explicit worker settings")
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

func TestServerLoadsLegacyCapacityAndFormerPreviewPort(t *testing.T) {
	path := filepath.Join(t.TempDir(), ".env")
	legacy := "ADMIN_PASSWORD=synthetic-admin\nLISTEN_ADDR=127.0.0.1:2050\n" +
		"WAA_BACKEND=spidermonkey\nWARM_WORKER_LIMIT=1\nMAX_ACTIVE_WORKERS=2\nWARM_STARTUP_CONCURRENCY=1\n"
	if err := os.WriteFile(path, []byte(legacy), 0600); err != nil {
		t.Fatal(err)
	}
	cfg, err := LoadConfig(path)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.ListenAddr != "127.0.0.1:2050" || len(cfg.Invalid) != 0 || !cfg.PreviewDisabled {
		t.Fatal("disabled preview changed a valid server listener")
	}
	if cfg.WarmWorkerLimit != 1 || cfg.MaxActiveWorkers != 2 || cfg.WarmStartupConcurrency != 1 {
		t.Fatal("explicit legacy capacity was overwritten")
	}
	if err := cfg.Save(path); err != nil {
		t.Fatal(err)
	}
	reloaded, err := LoadConfig(path)
	if err != nil {
		t.Fatal(err)
	}
	if reloaded.ListenAddr != cfg.ListenAddr || reloaded.WarmWorkerLimit != 1 || reloaded.MaxActiveWorkers != 2 || reloaded.WarmStartupConcurrency != 1 {
		t.Fatal("explicit legacy capacity did not survive a configuration save and reload")
	}
}

func TestServerWorkerDefaultsAndOverridesRoundTrip(t *testing.T) {
	for _, test := range []struct {
		name    string
		content string
		warm    int
		active  int
		startup int
	}{
		{"defaults", "", 1, 4, 4},
		{"lower", "WARM_WORKER_LIMIT=2\nMAX_ACTIVE_WORKERS=4\nWARM_STARTUP_CONCURRENCY=2\n", 2, 4, 2},
		{"higher", "WARM_WORKER_LIMIT=3\nMAX_ACTIVE_WORKERS=8\nWARM_STARTUP_CONCURRENCY=6\n", 3, 8, 6},
		{"old_upstream", "WARM_WORKER_LIMIT=10\nMAX_ACTIVE_WORKERS=20\nWARM_STARTUP_CONCURRENCY=5\n", 10, 20, 5},
	} {
		t.Run(test.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), ".env")
			if err := os.WriteFile(path, []byte("WAA_BACKEND=spidermonkey\nADMIN_PASSWORD=synthetic-admin\n"+test.content), 0600); err != nil {
				t.Fatal(err)
			}
			for range 2 {
				cfg, err := LoadConfig(path)
				if err != nil {
					t.Fatal(err)
				}
				if len(cfg.Invalid) != 0 || cfg.WarmWorkerLimit != test.warm || cfg.MaxActiveWorkers != test.active || cfg.WarmStartupConcurrency != test.startup {
					t.Fatal("load/save changed valid worker settings")
				}
				if err := cfg.Save(path); err != nil {
					t.Fatal(err)
				}
			}
			if test.name == "defaults" {
				data, err := os.ReadFile(path)
				if err != nil {
					t.Fatal(err)
				}
				for _, key := range []string{"WARM_WORKER_LIMIT=", "MAX_ACTIVE_WORKERS=", "WARM_STARTUP_CONCURRENCY="} {
					if strings.Contains(string(data), key) {
						t.Fatal("saving defaults no longer omits default worker values")
					}
				}
			}
		})
	}
}

func TestServerUsesActualBackendForWorkerDefaults(t *testing.T) {
	for _, test := range []struct {
		name    string
		source  string
		content string
		warm    int
		active  int
		startup int
	}{
		{"file_defaults", "file", "", 1, 4, 4},
		{"environment_defaults", "environment", "", 1, 4, 4},
		{"detected_accounts_defaults", "accounts", "", 1, 4, 4},
		{"explicit_lower", "file", "WARM_WORKER_LIMIT=1\nMAX_ACTIVE_WORKERS=2\nWARM_STARTUP_CONCURRENCY=2\n", 1, 2, 2},
		{"explicit_higher", "file", "WARM_WORKER_LIMIT=3\nMAX_ACTIVE_WORKERS=8\nWARM_STARTUP_CONCURRENCY=6\n", 3, 8, 6},
	} {
		t.Run(test.name, func(t *testing.T) {
			root := t.TempDir()
			content := "ADMIN_PASSWORD=synthetic-admin\n" + test.content
			switch test.source {
			case "file":
				content += "WAA_BACKEND=camoufox\n"
			case "environment":
				t.Setenv("WAA_BACKEND", "camoufox")
			case "accounts":
				authRoot := filepath.Join(root, "auth")
				accountRoot := filepath.Join(authRoot, "synthetic")
				if err := os.MkdirAll(accountRoot, 0700); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(filepath.Join(accountRoot, "account.json"), []byte("{}"), 0600); err != nil {
					t.Fatal(err)
				}
				content += "AISTUDIO_AUTH_STATES=" + authRoot + "\n"
			}
			path := filepath.Join(root, ".env")
			if err := os.WriteFile(path, []byte(content), 0600); err != nil {
				t.Fatal(err)
			}
			for range 2 {
				cfg, err := LoadConfig(path)
				if err != nil {
					t.Fatal(err)
				}
				if cfg.WAABackend != config.WAABackendSpiderMonkey || cfg.WarmWorkerLimit != test.warm || cfg.MaxActiveWorkers != test.active || cfg.WarmStartupConcurrency != test.startup {
					t.Fatalf("backend=%s warm=%d active=%d startup=%d", cfg.WAABackend, cfg.WarmWorkerLimit, cfg.MaxActiveWorkers, cfg.WarmStartupConcurrency)
				}
				if err := cfg.Save(path); err != nil {
					t.Fatal(err)
				}
			}
		})
	}
}
