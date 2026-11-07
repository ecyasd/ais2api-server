package config

import "testing"

func TestDisabledPreviewDoesNotReserveItsPort(t *testing.T) {
	for _, source := range []string{"file", "environment"} {
		t.Run(source, func(t *testing.T) {
			content := "WAA_BACKEND=spidermonkey\n"
			if source == "environment" {
				t.Setenv("LISTEN_ADDR", "127.0.0.1:2050")
			} else {
				content += "LISTEN_ADDR=127.0.0.1:2050\n"
			}
			path := writeTestEnv(t, content)
			cfg, err := LoadWithOptions(path, LoadOptions{DisablePreview: true})
			if err != nil {
				t.Fatal(err)
			}
			if cfg.ListenAddr != "127.0.0.1:2050" || len(cfg.Invalid) != 0 {
				t.Fatal("disabled preview caused a listener fallback")
			}
			if err := cfg.Save(path); err != nil {
				t.Fatal(err)
			}
			cfg.ListenAddr = "invalid-listener"
			if cfg.Validate() == nil {
				t.Fatal("disabling preview also disabled main listener validation")
			}
		})
	}
}

func TestOrdinaryConfigKeepsPreviewValidation(t *testing.T) {
	cfg := Default()
	cfg.ListenAddr = cfg.PreviewListenAddr
	if cfg.Validate() == nil {
		t.Fatal("ordinary configuration accepted conflicting listeners")
	}
	path := writeTestEnv(t, "WAA_BACKEND=spidermonkey\nLISTEN_ADDR=127.0.0.1:2050\n")
	loaded, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.ListenAddr != defaultListenAddr || len(loaded.Invalid) == 0 || loaded.PreviewDisabled {
		t.Fatal("ordinary load no longer preserves upstream validation and fallback")
	}
}

func TestDisabledPreviewIgnoresOnlyItsOwnAddress(t *testing.T) {
	path := writeTestEnv(t, "WAA_BACKEND=spidermonkey\nPREVIEW_LISTEN_ADDR=unused-invalid-address\n")
	cfg, err := LoadWithOptions(path, LoadOptions{DisablePreview: true})
	if err != nil {
		t.Fatal(err)
	}
	if cfg.PreviewListenAddr != "unused-invalid-address" || len(cfg.Invalid) != 0 {
		t.Fatal("disabled preview address was still validated")
	}
	cfg.PreviewDisabled = false
	if cfg.Validate() == nil {
		t.Fatal("enabled preview accepted an invalid listener")
	}
}
