package agent

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	boxlite "github.com/RussellLuo/boxlite/sdks/go"

	"csgclaw/internal/config"
)

func (s *Service) createGatewayBox(ctx context.Context, rt *boxlite.Runtime, image, name, botID, modelID string) (*boxlite.Box, *boxlite.BoxInfo, error) {
	if testCreateGatewayBoxHook != nil {
		return testCreateGatewayBoxHook(s, ctx, rt, image, name, botID, modelID)
	}
	if !runtimeValid(rt) {
		return nil, nil, fmt.Errorf("invalid boxlite runtime")
	}
	boxOpts, err := s.gatewayBoxOptions(name, botID, modelID)
	if err != nil {
		return nil, nil, err
	}
	box, err := rt.Create(ctx, image, boxOpts...)
	if err != nil {
		return nil, nil, fmt.Errorf("create gateway box: %w", err)
	}
	if err := box.Start(ctx); err != nil {
		_ = s.closeBox(box)
		return nil, nil, fmt.Errorf("start gateway box: %w", err)
	}
	info, err := box.Info(ctx)
	if err != nil {
		_ = s.closeBox(box)
		return nil, nil, fmt.Errorf("read gateway box info: %w", err)
	}
	return box, info, nil
}

func (s *Service) forceRemoveBox(ctx context.Context, rt *boxlite.Runtime, idOrName string) error {
	if testForceRemoveBoxHook != nil {
		return testForceRemoveBoxHook(s, ctx, rt, idOrName)
	}
	if !runtimeValid(rt) {
		return fmt.Errorf("invalid boxlite runtime")
	}
	return rt.ForceRemove(ctx, idOrName)
}

func (s *Service) gatewayBoxOptions(name, botID, modelID string) ([]boxlite.BoxOption, error) {
	if strings.TrimSpace(modelID) == "" {
		modelID = s.llm.ModelID
	}
	envVars := picoclawBoxEnvVars(resolveManagerBaseURL(s.server), s.server.AccessToken, botID, s.llm)
	opts := []boxlite.BoxOption{
		boxlite.WithName(name),
		boxlite.WithDetach(true),
		boxlite.WithAutoRemove(false),
		//boxlite.WithPort(managerHostPort, managerGuestPort),
		boxlite.WithEnv("HOME", "/home/picoclaw"),
		boxlite.WithEnv("CSGCLAW_LLM_BASE_URL", s.llm.BaseURL),
		boxlite.WithEnv("CSGCLAW_LLM_API_KEY", s.llm.APIKey),
		boxlite.WithEnv("CSGCLAW_LLM_MODEL_ID", modelID),
		boxlite.WithEnv("OPENAI_BASE_URL", s.llm.BaseURL),
		boxlite.WithEnv("OPENAI_API_KEY", s.llm.APIKey),
		boxlite.WithEnv("OPENAI_MODEL", modelID),
	}
	for key, value := range envVars {
		opts = append(opts, boxlite.WithEnv(key, value))
	}
	//entrypoint, cmd := gatewayStartCommand(managerDebugMode)
	opts = append(opts,
		//boxlite.WithEntrypoint(entrypoint...),
		//boxlite.WithCmd(cmd...),
		boxlite.WithCmd("/bin/sh", "-c", "/usr/local/bin/picoclaw gateway -d 1>~/.picoclaw/gateway.log 2>/dev/null"),
		//boxlite.WithCmd("sleep", "infinity"),
	)

	//hostPicoClawRoot, err := ensureAgentPicoClawConfig(name, botID, s.server, s.llm)
	//if err != nil {
	//	return nil, err
	//}
	//opts = append(opts, boxlite.WithVolume(hostPicoClawRoot, boxPicoClawDir))
	projectsRoot, err := ensureAgentProjectsRoot()
	if err != nil {
		return nil, err
	}
	opts = append(opts, boxlite.WithVolume(projectsRoot, boxProjectsDir))

	return opts, nil
}

func gatewayStartCommand(debug bool) ([]string, []string) {
	if debug {
		return []string{"sleep"}, []string{"infinity"}
	}
	return []string{"tini"}, []string{"--", "picoclaw", "gateway", "-d"}
}

func ensureAgentProjectsRoot() (string, error) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("resolve host home dir: %w", err)
	}
	hostProjectsRoot := filepath.Join(homeDir, config.AppDirName, hostProjectsDir)
	if err := os.MkdirAll(hostProjectsRoot, 0o755); err != nil {
		return "", fmt.Errorf("create host projects dir: %w", err)
	}
	return hostProjectsRoot, nil
}

func picoclawBoxEnvVars(baseURL, accessToken, botID string, llm config.LLMConfig) map[string]string {
	return map[string]string{
		"CSGCLAW_BASE_URL":                       baseURL,
		"CSGCLAW_ACCESS_TOKEN":                   accessToken,
		"PICOCLAW_CHANNELS_CSGCLAW_BASE_URL":     baseURL,
		"PICOCLAW_CHANNELS_CSGCLAW_ACCESS_TOKEN": accessToken,
		"PICOCLAW_CHANNELS_CSGCLAW_BOT_ID":       botID,
		"PICOCLAW_AGENTS_DEFAULTS_MODEL_NAME":    llm.ModelID,
		"PICOCLAW_CUSTOM_MODEL_NAME":             llm.ModelID,
		"PICOCLAW_CUSTOM_MODEL_ID":               llm.ModelID,
		"PICOCLAW_CUSTOM_MODEL_API_KEY":          llm.APIKey,
		"PICOCLAW_CUSTOM_MODEL_BASE_URL":         llm.BaseURL,
	}
}
