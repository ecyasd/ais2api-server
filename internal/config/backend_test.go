package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestOrdinaryLoadKeepsCamoufoxBackendDefaults(t *testing.T) {
	for _, source := range []string{"file", "accounts"} {
		t.Run(source, func(t *testing.T) {
			content := "WAA_BACKEND=camoufox\n"
			if source == "accounts" {
				authRoot := t.TempDir()
				accountRoot := filepath.Join(authRoot, "synthetic")
				if err := os.Mkdir(accountRoot, 0700); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(filepath.Join(accountRoot, "account.json"), []byte("{}"), 0600); err != nil {
					t.Fatal(err)
				}
				content = "AISTUDIO_AUTH_STATES=" + authRoot + "\n"
			}
			cfg, err := Load(writeTestEnv(t, content))
			if err != nil {
				t.Fatal(err)
			}
			if cfg.WAABackend != WAABackendCamoufox || cfg.WarmWorkerLimit != camoufoxWarmWorkerLimit || cfg.MaxActiveWorkers != camoufoxMaxActiveWorkers {
				t.Fatal("ordinary loading lost its backend selection or defaults")
			}
		})
	}
}

func TestLoadRejectsInvalidForcedBackend(t *testing.T) {
	if _, err := LoadWithOptions(writeTestEnv(t, ""), LoadOptions{ForceWAABackend: "invalid"}); err == nil {
		t.Fatal("invalid assembly backend was accepted")
	}
}
