package config

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func tempConfig(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	t.Setenv("TROPMAIL_CONFIG_DIR", dir)
	return dir
}

func TestLoadMissingFileYieldsEmptyConfig(t *testing.T) {
	tempConfig(t)

	cfg, err := Load()
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if len(cfg.Profiles) != 0 {
		t.Errorf("profiles = %v, want empty", cfg.Profiles)
	}
	if got := cfg.ResolveName(""); got != DefaultProfile {
		t.Errorf("ResolveName = %q, want %q", got, DefaultProfile)
	}
}

func TestSaveAndReload(t *testing.T) {
	dir := tempConfig(t)

	cfg, err := Load()
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	cfg.Set("work", Profile{BaseURL: "https://example.test/api/v1", Email: "a@b.c", Tier: "Pro"})
	if err := cfg.Save(); err != nil {
		t.Fatalf("save: %v", err)
	}

	info, err := os.Stat(filepath.Join(dir, "config.toml"))
	if err != nil {
		t.Fatalf("stat: %v", err)
	}
	if mode := info.Mode().Perm(); mode != 0o600 {
		t.Errorf("config mode = %o, want 600", mode)
	}

	reloaded, err := Load()
	if err != nil {
		t.Fatalf("reload: %v", err)
	}
	if got := reloaded.Get("work").Email; got != "a@b.c" {
		t.Errorf("email = %q", got)
	}
	// The first profile saved becomes the default.
	if reloaded.DefaultProfile != "work" {
		t.Errorf("default = %q, want work", reloaded.DefaultProfile)
	}
}

func TestResolveNamePrecedence(t *testing.T) {
	cfg := &Config{DefaultProfile: "configured", Profiles: map[string]Profile{}}

	t.Setenv("TROPMAIL_PROFILE", "from-env")
	if got := cfg.ResolveName("explicit"); got != "explicit" {
		t.Errorf("explicit lost: %q", got)
	}
	if got := cfg.ResolveName(""); got != "from-env" {
		t.Errorf("env ignored: %q", got)
	}

	t.Setenv("TROPMAIL_PROFILE", "")
	if got := cfg.ResolveName(""); got != "configured" {
		t.Errorf("configured default ignored: %q", got)
	}
}

func TestRemoveReassignsDefault(t *testing.T) {
	cfg := &Config{Profiles: map[string]Profile{}}
	cfg.Set("first", Profile{})
	cfg.Set("second", Profile{})

	if err := cfg.Remove("first"); err != nil {
		t.Fatalf("remove: %v", err)
	}
	if cfg.DefaultProfile != "second" {
		t.Errorf("default = %q, want second", cfg.DefaultProfile)
	}
	if err := cfg.Remove("missing"); !errors.Is(err, ErrNoProfile) {
		t.Errorf("err = %v, want ErrNoProfile", err)
	}
}

func TestEndpointFallsBackToProduction(t *testing.T) {
	if got := (Profile{}).Endpoint(); got != DefaultBaseURL {
		t.Errorf("Endpoint = %q", got)
	}
	if got := (Profile{BaseURL: "http://local"}).Endpoint(); got != "http://local" {
		t.Errorf("Endpoint = %q", got)
	}
}

func TestEnvironmentKeyWinsOverStorage(t *testing.T) {
	tempConfig(t)
	t.Setenv("TROPMAIL_API_KEY", "envkeyenvkeyenvkeyenvkeyenvkey12")

	key, storage, err := LoadKey("default")
	if err != nil {
		t.Fatalf("load key: %v", err)
	}
	if storage != StorageEnvironment || key != "envkeyenvkeyenvkeyenvkeyenvkey12" {
		t.Errorf("key = %q from %q", key, storage)
	}
}

func TestFileCredentialsRoundTrip(t *testing.T) {
	dir := tempConfig(t)
	t.Setenv("TROPMAIL_API_KEY", "")

	const key = "filekeyfilekeyfilekeyfilekeyfile"
	if err := storeKeyInFile("default", key); err != nil {
		t.Fatalf("store: %v", err)
	}

	info, err := os.Stat(filepath.Join(dir, "credentials.toml"))
	if err != nil {
		t.Fatalf("stat: %v", err)
	}
	if mode := info.Mode().Perm(); mode != 0o600 {
		t.Errorf("credentials mode = %o, want 600", mode)
	}

	// A keyring may exist on the test machine and would shadow the file, so
	// only assert the file path when the keyring lookup misses.
	loaded, storage, err := LoadKey("default")
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if storage == StorageFile && loaded != key {
		t.Errorf("key = %q, want %q", loaded, key)
	}
}

func TestLoadKeyWithoutCredentials(t *testing.T) {
	tempConfig(t)
	t.Setenv("TROPMAIL_API_KEY", "")

	if _, _, err := LoadKey("nonexistent-profile-xyz"); err == nil {
		t.Fatal("expected an error for a profile with no key")
	}
}
