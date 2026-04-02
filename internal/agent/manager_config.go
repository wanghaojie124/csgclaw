package agent

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strings"

	"csgclaw/internal/config"
)

const managerAgentsDirName = "agents"

//go:embed defaults/picoclaw-config.json
var defaultManagerPicoClawConfig []byte

//go:embed defaults/manager-security.yml
var defaultManagerSecurityConfig string

func ensureManagerPicoClawConfig(server config.ServerConfig, llm config.LLMConfig, pico config.PicoClawConfig) (string, error) {
	return ensureAgentPicoClawConfig(ManagerName, "u-manager", server, llm, pico)
}

func ensureAgentPicoClawConfig(agentName, botID string, server config.ServerConfig, llm config.LLMConfig, pico config.PicoClawConfig) (string, error) {
	hostRoot, err := agentPicoClawRoot(agentName)
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(filepath.Join(hostRoot, hostPicoClawLogs), 0o755); err != nil {
		return "", fmt.Errorf("create manager picoclaw logs dir: %w", err)
	}

	data, err := renderAgentPicoClawConfig(botID, server, llm, pico)
	if err != nil {
		return "", err
	}
	configPath := filepath.Join(hostRoot, hostPicoClawConfig)
	if err := os.WriteFile(configPath, append(data, '\n'), 0o600); err != nil {
		return "", fmt.Errorf("write manager picoclaw config: %w", err)
	}
	securityData := renderManagerSecurityConfig(llm)
	securityPath := filepath.Join(hostRoot, ".security.yml")
	if err := os.WriteFile(securityPath, []byte(securityData), 0o600); err != nil {
		return "", fmt.Errorf("write manager security config: %w", err)
	}
	return hostRoot, nil
}

func managerPicoClawRoot() (string, error) {
	return agentPicoClawRoot(ManagerName)
}

func agentPicoClawRoot(agentName string) (string, error) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("resolve host home dir: %w", err)
	}
	return filepath.Join(homeDir, config.AppDirName, managerAgentsDirName, agentName, hostPicoClawDir), nil
}

func renderManagerPicoClawConfig(server config.ServerConfig, llm config.LLMConfig, pico config.PicoClawConfig) ([]byte, error) {
	return renderAgentPicoClawConfig("u-manager", server, llm, pico)
}

func renderAgentPicoClawConfig(botID string, server config.ServerConfig, llm config.LLMConfig, pico config.PicoClawConfig) ([]byte, error) {
	var cfg map[string]any
	if err := json.Unmarshal(defaultManagerPicoClawConfig, &cfg); err != nil {
		return nil, fmt.Errorf("decode embedded manager picoclaw config: %w", err)
	}

	if err := updateModelList(cfg, llm); err != nil {
		return nil, err
	}
	if err := updateCSGClawChannel(cfg, botID, server, pico); err != nil {
		return nil, err
	}

	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("encode manager picoclaw config: %w", err)
	}
	return data, nil
}

func updateModelList(cfg map[string]any, llm config.LLMConfig) error {
	modelList, ok := cfg["model_list"].([]any)
	if !ok || len(modelList) == 0 {
		return fmt.Errorf("embedded manager picoclaw config is missing model_list[0]")
	}
	model, ok := modelList[0].(map[string]any)
	if !ok {
		return fmt.Errorf("embedded manager picoclaw config has invalid model_list[0]")
	}
	if llm.ModelID != "" {
		model["model_name"] = llm.ModelID
		model["model"] = llm.ModelID
	}
	if llm.BaseURL != "" {
		model["api_base"] = strings.TrimRight(llm.BaseURL, "/")
	}
	if llm.APIKey != "" {
		model["api_key"] = llm.APIKey
	}
	return nil
}

func updateCSGClawChannel(cfg map[string]any, botID string, server config.ServerConfig, pico config.PicoClawConfig) error {
	channels, ok := cfg["channels"].(map[string]any)
	if !ok {
		return fmt.Errorf("embedded manager picoclaw config is missing channels")
	}
	channel, ok := channels["csgclaw"].(map[string]any)
	if !ok {
		return fmt.Errorf("embedded manager picoclaw config is missing channels.csgclaw")
	}
	if baseURL := resolveManagerBaseURL(server); baseURL != "" {
		channel["base_url"] = baseURL
	}
	if pico.AccessToken != "" {
		channel["access_token"] = pico.AccessToken
	}
	channel["bot_id"] = botID
	channel["enabled"] = true
	return nil
}

func resolveManagerBaseURL(server config.ServerConfig) string {
	port := "18080"
	if server.ListenAddr != "" {
		if _, resolvedPort, err := net.SplitHostPort(server.ListenAddr); err == nil && resolvedPort != "" {
			port = resolvedPort
		}
	}
	if ip := localIPv4Resolver(); ip != "" {
		return fmt.Sprintf("http://%s:%s", ip, port)
	}
	if server.APIBaseURL != "" {
		return strings.TrimRight(server.APIBaseURL, "/")
	}
	return ""
}

func localIPv4() string {
	if ip := outboundIPv4(); ip != "" {
		return ip
	}
	return interfaceIPv4()
}

func outboundIPv4() string {
	conn, err := net.Dial("udp4", "8.8.8.8:80")
	if err != nil {
		return ""
	}
	defer conn.Close()

	addr, ok := conn.LocalAddr().(*net.UDPAddr)
	if !ok || addr.IP == nil {
		return ""
	}
	ip := addr.IP.To4()
	if ip == nil || ip.IsLoopback() || ip.IsUnspecified() {
		return ""
	}
	return ip.String()
}

func interfaceIPv4() string {
	ifaces, err := net.Interfaces()
	if err != nil {
		return ""
	}
	for _, iface := range ifaces {
		if iface.Flags&net.FlagUp == 0 || iface.Flags&net.FlagLoopback != 0 {
			continue
		}
		addrs, err := iface.Addrs()
		if err != nil {
			continue
		}
		for _, addr := range addrs {
			if ip := ipv4FromAddr(addr); ip != "" {
				return ip
			}
		}
	}
	return ""
}

func ipv4FromAddr(addr net.Addr) string {
	switch v := addr.(type) {
	case *net.IPNet:
		ip := v.IP.To4()
		if ip == nil || ip.IsLoopback() || ip.IsUnspecified() {
			return ""
		}
		return ip.String()
	case *net.IPAddr:
		ip := v.IP.To4()
		if ip == nil || ip.IsLoopback() || ip.IsUnspecified() {
			return ""
		}
		return ip.String()
	default:
		return ""
	}
}

func renderManagerSecurityConfig(llm config.LLMConfig) string {
	modelID := llm.ModelID
	if modelID == "" {
		modelID = config.DefaultLLMModelID
	}
	apiKey := llm.APIKey
	if apiKey == "" {
		apiKey = config.DefaultLLMAPIKey
	}

	content := strings.ReplaceAll(defaultManagerSecurityConfig, "__MODEL_ID__", modelID)
	content = strings.ReplaceAll(content, "__API_KEY__", apiKey)
	if !strings.HasSuffix(content, "\n") {
		content += "\n"
	}
	return content
}
