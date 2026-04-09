package config

import (
	"os"
	"path/filepath"
	"strings"
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

func TestDefaultAgentsPathUsesDomainSubdirectory(t *testing.T) {
	path, err := DefaultAgentsPath()
	if err != nil {
		t.Fatalf("DefaultAgentsPath() error = %v", err)
	}

	if got, want := filepath.Base(path), StateFileName; got != want {
		t.Fatalf("filepath.Base(DefaultAgentsPath()) = %q, want %q", got, want)
	}
	if got, want := filepath.Base(filepath.Dir(path)), AgentsDirName; got != want {
		t.Fatalf("filepath.Base(filepath.Dir(DefaultAgentsPath())) = %q, want %q", got, want)
	}
}

func TestDefaultIMStatePathUsesDomainSubdirectory(t *testing.T) {
	path, err := DefaultIMStatePath()
	if err != nil {
		t.Fatalf("DefaultIMStatePath() error = %v", err)
	}

	if got, want := filepath.Base(path), StateFileName; got != want {
		t.Fatalf("filepath.Base(DefaultIMStatePath()) = %q, want %q", got, want)
	}
	if got, want := filepath.Base(filepath.Dir(path)), IMDirName; got != want {
		t.Fatalf("filepath.Base(filepath.Dir(DefaultIMStatePath())) = %q, want %q", got, want)
	}
}

func TestLoadAppliesDefaultManagerImage(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.toml")
	content := `[server]
listen_addr = "127.0.0.1:18080"
advertise_base_url = "http://127.0.0.1:18080"

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
	if got, want := cfg.Server.AccessToken, DefaultAccessToken; got != want {
		t.Fatalf("cfg.Server.AccessToken = %q, want %q", got, want)
	}
}

func TestSaveWritesAccessTokenUnderServerSection(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.toml")
	cfg := Config{
		Server: ServerConfig{
			ListenAddr:       "127.0.0.1:18080",
			AdvertiseBaseURL: "http://127.0.0.1:18080",
			AccessToken:      "shared-token",
		},
		LLM: LLMConfig{
			BaseURL: "http://127.0.0.1:4000",
			APIKey:  "sk",
			ModelID: "minimax-m2.7",
		},
		Bootstrap: BootstrapConfig{
			ManagerImage: "img",
		},
	}

	if err := cfg.Save(path); err != nil {
		t.Fatalf("Save() error = %v", err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	content := string(data)
	if !strings.Contains(content, "access_token = \"shared-token\"") {
		t.Fatalf("saved config missing server access token:\n%s", content)
	}
	if strings.Contains(content, "[picoclaw]") {
		t.Fatalf("saved config should not contain [picoclaw] section:\n%s", content)
	}
}
