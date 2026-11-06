package app

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/Mag1cFall/AIStudio2API/internal/aistudio"
	"github.com/Mag1cFall/AIStudio2API/internal/config"
	"github.com/Mag1cFall/AIStudio2API/internal/product"
)

func TestServerWorkerManagerUsesConfiguredStartupSlots(t *testing.T) {
	for _, test := range []struct {
		name    string
		content string
		warm    int
		active  int
		startup int
	}{
		{"defaults", "", 1, 4, 4},
		{"legacy", "WARM_WORKER_LIMIT=1\nMAX_ACTIVE_WORKERS=2\nWARM_STARTUP_CONCURRENCY=1\n", 1, 2, 1},
		{"lower", "WARM_WORKER_LIMIT=2\nMAX_ACTIVE_WORKERS=4\nWARM_STARTUP_CONCURRENCY=2\n", 2, 4, 2},
		{"higher", "WARM_WORKER_LIMIT=3\nMAX_ACTIVE_WORKERS=8\nWARM_STARTUP_CONCURRENCY=6\n", 3, 8, 6},
	} {
		t.Run(test.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), ".env")
			if err := os.WriteFile(path, []byte("WAA_BACKEND=spidermonkey\n"+test.content), 0600); err != nil {
				t.Fatal(err)
			}
			cfg, err := product.LoadConfig(path)
			if err != nil {
				t.Fatal(err)
			}
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			pool := aistudio.NewAccountPool(nil, cfg.PerAccountConcurrency)
			manager := newAccountWorkerManager(pool, nil, newRequestRegistry(ctx), "", "", time.Second,
				cfg.WarmWorkerLimit, cfg.MaxActiveWorkers, cfg.WarmStartupConcurrency)
			defer manager.Close()
			if manager.warmLimit() != test.warm || manager.activeLimit() != test.active || manager.startup.size() != test.startup {
				t.Fatal("actual worker manager did not apply the configured limits")
			}
			if len(manager.WarmAccountIDs()) != 0 {
				t.Fatal("assembling the manager started workers")
			}
			acquired := 0
			defer func() {
				for range acquired {
					manager.startup.release()
				}
			}()
			for range test.startup {
				slotCtx, stop := context.WithTimeout(ctx, time.Second)
				err := manager.startup.acquire(slotCtx, "")
				stop()
				if err != nil {
					t.Fatalf("startup slot %d was unavailable: %v", acquired+1, err)
				}
				acquired++
			}
			blocked, stop := context.WithTimeout(ctx, 20*time.Millisecond)
			defer stop()
			if err := manager.startup.acquire(blocked, ""); !errors.Is(err, context.DeadlineExceeded) {
				if err == nil {
					acquired++
				}
				t.Fatal("a concurrent startup above the configured limit was admitted")
			}
		})
	}
}

func TestWorkerStartupLimitIsIndependentOfActiveCapacity(t *testing.T) {
	for _, executable := range []string{"", "synthetic-camoufox"} {
		manager := &accountWorkerManager{camoufox: executable}
		manager.setLimits(1, 8, 4)
		if manager.startup.size() != 4 {
			t.Fatal("worker backend replaced the configured startup limit with the active limit")
		}
	}
}

func TestServerCLIAllowsFormerPreviewPort(t *testing.T) {
	cfg := product.Constrain(config.Default())
	cfg.AdminPassword = "synthetic-admin"
	if _, err := parseFlags([]string{"-open-ui=false", "-listen", "127.0.0.1:2050"}, &cfg); err != nil {
		t.Fatal(err)
	}
	if cfg.ListenAddr != "127.0.0.1:2050" {
		t.Fatal("CLI listener was changed")
	}
}

func TestServerAdminAllowsFormerPreviewPortAcrossReloads(t *testing.T) {
	path := filepath.Join(t.TempDir(), ".env")
	content := "WAA_BACKEND=spidermonkey\nADMIN_AUTH_ENABLED=true\nADMIN_PASSWORD=synthetic-admin\nLISTEN_ADDR=127.0.0.1:2050\n"
	if err := os.WriteFile(path, []byte(content), 0600); err != nil {
		t.Fatal(err)
	}
	cfg, err := product.LoadConfig(path)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	pool := aistudio.NewAccountPool(nil, 1)
	admin := &runtimeAdmin{configPath: path, config: cfg, requests: newRequestRegistry(ctx), pool: pool, service: &trackedService{pool: pool}}
	value, err := admin.RuntimeConfig(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if value.ListenAddr != "127.0.0.1:2050" {
		t.Fatal("management load replaced the configured listener")
	}
	for _, settings := range []struct {
		address string
		warm    int
		active  int
		startup int
	}{
		{"127.0.0.1:2048", 2, 4, 2},
		{"127.0.0.1:2050", 3, 8, 6},
		{"127.0.0.1:2050", 1, 4, 4},
	} {
		value.ListenAddr = settings.address
		value.WarmWorkerLimit = settings.warm
		value.MaxActiveWorkers = settings.active
		value.WarmStartupConcurrency = settings.startup
		if _, err := admin.UpdateRuntimeConfig(ctx, value); err != nil {
			t.Fatal(err)
		}
		loaded, err := admin.RuntimeConfig(ctx)
		if err != nil {
			t.Fatal(err)
		}
		if loaded.ListenAddr != settings.address || loaded.WarmWorkerLimit != settings.warm || loaded.MaxActiveWorkers != settings.active || loaded.WarmStartupConcurrency != settings.startup {
			t.Fatal("management save/reload replaced the configured listener or worker limits")
		}
	}
}

func TestServerAdminWorkerSettingsKeepEnvironmentOverrides(t *testing.T) {
	path := filepath.Join(t.TempDir(), ".env")
	content := "WAA_BACKEND=spidermonkey\nADMIN_PASSWORD=synthetic-admin\n" +
		"WARM_WORKER_LIMIT=1\nMAX_ACTIVE_WORKERS=4\nWARM_STARTUP_CONCURRENCY=2\n"
	if err := os.WriteFile(path, []byte(content), 0600); err != nil {
		t.Fatal(err)
	}
	t.Run("environment_wins_over_management", func(t *testing.T) {
		t.Setenv("WARM_WORKER_LIMIT", "3")
		t.Setenv("MAX_ACTIVE_WORKERS", "8")
		t.Setenv("WARM_STARTUP_CONCURRENCY", "6")
		cfg, err := product.LoadConfig(path)
		if err != nil {
			t.Fatal(err)
		}
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		pool := aistudio.NewAccountPool(nil, 1)
		admin := &runtimeAdmin{configPath: path, config: cfg, requests: newRequestRegistry(ctx), pool: pool, service: &trackedService{pool: pool}}
		value, err := admin.RuntimeConfig(ctx)
		if err != nil {
			t.Fatal(err)
		}
		if value.WarmWorkerLimit != 3 || value.MaxActiveWorkers != 8 || value.WarmStartupConcurrency != 6 {
			t.Fatal("environment did not override the file's worker limits")
		}
		value.WarmWorkerLimit, value.MaxActiveWorkers, value.WarmStartupConcurrency = 1, 4, 2
		updated, err := admin.UpdateRuntimeConfig(ctx, value)
		if err != nil {
			t.Fatal(err)
		}
		if updated.WarmWorkerLimit != 3 || updated.MaxActiveWorkers != 8 || updated.WarmStartupConcurrency != 6 {
			t.Fatal("management settings overrode environment worker limits")
		}
	})
	reloaded, err := product.LoadConfig(path)
	if err != nil {
		t.Fatal(err)
	}
	if reloaded.WarmWorkerLimit != 1 || reloaded.MaxActiveWorkers != 4 || reloaded.WarmStartupConcurrency != 2 {
		t.Fatal("saving under environment overrides changed the underlying file's worker settings")
	}
}
