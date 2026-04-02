package agent

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	boxlite "github.com/RussellLuo/boxlite/sdks/go"

	"csgclaw/internal/config"
)

func (s *Service) ensureRuntime(agentName string) (*boxlite.Runtime, error) {
	if testEnsureRuntimeHook != nil {
		return testEnsureRuntimeHook(s, agentName)
	}
	s.mu.Lock()
	defer s.mu.Unlock()

	agentName = strings.TrimSpace(agentName)
	if agentName == "" {
		return nil, fmt.Errorf("agent name is required")
	}
	if rt := s.runtimes[agentName]; rt != nil {
		return rt, nil
	}

	homeDir, err := boxRuntimeHome(agentName)
	if err != nil {
		return nil, err
	}

	opts := []boxlite.RuntimeOption{boxlite.WithHomeDir(homeDir)}
	rt, err := boxlite.NewRuntime(opts...)
	if err != nil {
		return nil, fmt.Errorf("create boxlite runtime: %w", err)
	}
	s.runtimes[agentName] = rt
	return rt, nil
}

func boxRuntimeHome(agentName string) (string, error) {
	agentHome, err := agentHomeDir(agentName)
	if err != nil {
		return "", err
	}
	return filepath.Join(agentHome, config.RuntimeHomeDirName), nil
}

func agentHomeDir(agentName string) (string, error) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("resolve host home dir: %w", err)
	}
	return filepath.Join(homeDir, config.AppDirName, managerAgentsDirName, agentName), nil
}
