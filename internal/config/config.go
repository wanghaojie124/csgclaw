package config

import (
	"bufio"
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

type Config struct {
	Server    ServerConfig
	Model     ModelConfig
	Bootstrap BootstrapConfig
	Channels  ChannelsConfig
}

type ServerConfig struct {
	ListenAddr       string
	AdvertiseBaseURL string
	AccessToken      string
}

type ModelConfig struct {
	BaseURL string
	APIKey  string
	ModelID string
}

type BootstrapConfig struct {
	ManagerImage string
}

type ChannelsConfig struct {
	FeishuAdminOpenID string
	Feishu            map[string]FeishuConfig
}

type FeishuConfig struct {
	AppID     string
	AppSecret string
}

const (
	AppDirName         = ".csgclaw"
	RuntimeHomeDirName = "boxlite"
	ConfigFileName     = "config.toml"
	StateFileName      = "state.json"
	AgentsDirName      = "agents"
	IMDirName          = "im"
	ChannelsDirName    = "channels"

	DefaultHTTPPort     = "18080"
	DefaultAccessToken  = "your_access_token"
	DefaultManagerImage = "ghcr.io/russellluo/picoclaw:2026.4.8.1"
)

func DefaultListenAddr() string {
	return net.JoinHostPort("0.0.0.0", DefaultHTTPPort)
}

func DefaultAPIBaseURL() string {
	return "http://" + net.JoinHostPort("127.0.0.1", DefaultHTTPPort)
}

func ListenPort(listenAddr string) string {
	if listenAddr == "" {
		return DefaultHTTPPort
	}

	_, port, err := net.SplitHostPort(listenAddr)
	if err != nil || port == "" {
		return DefaultHTTPPort
	}
	return port
}

func (c ModelConfig) MissingFields() []string {
	var missing []string
	if strings.TrimSpace(c.BaseURL) == "" {
		missing = append(missing, "base_url")
	}
	if strings.TrimSpace(c.APIKey) == "" {
		missing = append(missing, "api_key")
	}
	if strings.TrimSpace(c.ModelID) == "" {
		missing = append(missing, "model_id")
	}
	return missing
}

func DefaultDir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("resolve home dir: %w", err)
	}
	return filepath.Join(home, AppDirName), nil
}

func DefaultPath() (string, error) {
	dir, err := DefaultDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, ConfigFileName), nil
}

func DefaultDomainDir(name string) (string, error) {
	dir, err := DefaultDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, name), nil
}

func DefaultAgentsDir() (string, error) {
	return DefaultDomainDir(AgentsDirName)
}

func DefaultIMDir() (string, error) {
	return DefaultDomainDir(IMDirName)
}

func DefaultAgentsPath() (string, error) {
	dir, err := DefaultAgentsDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, StateFileName), nil
}

func DefaultIMStatePath() (string, error) {
	dir, err := DefaultIMDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, StateFileName), nil
}

func DefaultChannelDir(name string) (string, error) {
	dir, err := DefaultDomainDir(ChannelsDirName)
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, name), nil
}

func LoadDefault() (Config, error) {
	path, err := DefaultPath()
	if err != nil {
		return Config{}, err
	}
	return Load(path)
}

func Load(path string) (Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return Config{}, fmt.Errorf("config not found at %s; run `csgclaw onboard` first", path)
		}
		return Config{}, fmt.Errorf("read config: %w", err)
	}

	cfg := Config{}
	section := ""
	scanner := bufio.NewScanner(strings.NewReader(string(data)))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		if strings.HasPrefix(line, "[") && strings.HasSuffix(line, "]") {
			section = strings.Trim(line, "[]")
			continue
		}

		key, value, ok := strings.Cut(line, "=")
		if !ok {
			return Config{}, fmt.Errorf("invalid line: %q", line)
		}

		key = strings.TrimSpace(key)
		value = strings.Trim(strings.TrimSpace(value), `"`)
		switch {
		case section == "server":
			switch key {
			case "listen_addr":
				cfg.Server.ListenAddr = value
			case "advertise_base_url":
				cfg.Server.AdvertiseBaseURL = strings.TrimRight(value, "/")
			case "access_token":
				cfg.Server.AccessToken = value
			}
		case section == "model":
			switch key {
			case "base_url":
				cfg.Model.BaseURL = value
			case "api_key":
				cfg.Model.APIKey = value
			case "model_id":
				cfg.Model.ModelID = value
			}
		case section == "bootstrap":
			switch key {
			case "manager_image":
				cfg.Bootstrap.ManagerImage = value
			}
		case section == "channels.feishu":
			switch key {
			case "admin_open_id":
				cfg.Channels.FeishuAdminOpenID = value
			}
		case strings.HasPrefix(section, "channels.feishu."):
			name := strings.TrimPrefix(section, "channels.feishu.")
			if name == "" {
				return Config{}, fmt.Errorf("invalid feishu channel section: %q", section)
			}
			if cfg.Channels.Feishu == nil {
				cfg.Channels.Feishu = make(map[string]FeishuConfig)
			}
			feishu := cfg.Channels.Feishu[name]
			switch key {
			case "app_id":
				feishu.AppID = value
			case "app_secret":
				feishu.AppSecret = value
			}
			cfg.Channels.Feishu[name] = feishu
		}
	}
	if err := scanner.Err(); err != nil {
		return Config{}, fmt.Errorf("scan config: %w", err)
	}

	if cfg.Server.ListenAddr == "" {
		cfg.Server.ListenAddr = DefaultListenAddr()
	}
	if cfg.Bootstrap.ManagerImage == "" {
		cfg.Bootstrap.ManagerImage = DefaultManagerImage
	}
	if cfg.Server.AccessToken == "" {
		cfg.Server.AccessToken = DefaultAccessToken
	}
	return cfg, nil
}

func (c Config) Save(path string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("create config dir: %w", err)
	}

	content := fmt.Sprintf(`# Generated by csgclaw onboard.

[server]
listen_addr = %q
advertise_base_url = %q
access_token = %q

[model]
base_url = %q
api_key = %q
model_id = %q

[bootstrap]
manager_image = %q
`, c.Server.ListenAddr, c.Server.AdvertiseBaseURL, c.Server.AccessToken, c.Model.BaseURL, c.Model.APIKey, c.Model.ModelID, c.Bootstrap.ManagerImage)

	if strings.TrimSpace(c.Channels.FeishuAdminOpenID) != "" {
		content += fmt.Sprintf(`
[channels.feishu]
admin_open_id = %q
`, c.Channels.FeishuAdminOpenID)
	}

	if len(c.Channels.Feishu) > 0 {
		names := make([]string, 0, len(c.Channels.Feishu))
		for name := range c.Channels.Feishu {
			names = append(names, name)
		}
		sort.Strings(names)
		for _, name := range names {
			feishu := c.Channels.Feishu[name]
			content += fmt.Sprintf(`
[channels.feishu.%s]
app_id = %q
app_secret = %q
`, name, feishu.AppID, feishu.AppSecret)
		}
	}

	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		return fmt.Errorf("write config: %w", err)
	}
	return nil
}
