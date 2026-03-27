package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDefaultDirUsesSharedAppDirName(t *testing.T) {
	dir, err := DefaultDir()
	if err != nil {
		t.Fatalf("DefaultDir() error = %v", err)
	}

	if got, want := filepath.Base(dir), AppDirName; got != want {
		t.Fatalf("filepath.Base(DefaultDir()) = %q, want %q", got, want)
	}
}

func TestDefaultRuntimeHomeUsesFixedBoxliteHome(t *testing.T) {
	runtimeHome, err := DefaultRuntimeHome()
	if err != nil {
		t.Fatalf("DefaultRuntimeHome() error = %v", err)
	}

	if got, want := filepath.Base(runtimeHome), RuntimeHomeDirName; got != want {
		t.Fatalf("filepath.Base(DefaultRuntimeHome()) = %q, want %q", got, want)
	}

	parent := filepath.Dir(runtimeHome)
	if got, want := filepath.Base(parent), AppDirName; got != want {
		t.Fatalf("filepath.Base(filepath.Dir(DefaultRuntimeHome())) = %q, want %q", got, want)
	}
}

func TestLoadAppliesDefaultManagerImage(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.toml")
	content := `[server]
listen_addr = "127.0.0.1:18080"
api_base_url = "http://127.0.0.1:18080"

[llm]
base_url = "http://127.0.0.1:4000"
api_key = "sk"
model_id = "minimax-m2.7"
`
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if got, want := cfg.Bootstrap.ManagerImage, DefaultManagerImage; got != want {
		t.Fatalf("cfg.Bootstrap.ManagerImage = %q, want %q", got, want)
	}
	if got, want := cfg.PicoClaw.AccessToken, DefaultPicoClawAccessToken; got != want {
		t.Fatalf("cfg.PicoClaw.AccessToken = %q, want %q", got, want)
	}
}
