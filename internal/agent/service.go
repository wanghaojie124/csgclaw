package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"slices"
	"sync"
	"time"

	boxlite "github.com/RussellLuo/boxlite/sdks/go"

	"csgclaw/internal/config"
)

type Agent struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	Image     string    `json:"image"`
	Status    string    `json:"status"`
	CreatedAt time.Time `json:"created_at"`
	ModelID   string    `json:"model_id,omitempty"`
}

type CreateRequest struct {
	Name  string `json:"name"`
	Image string `json:"image"`
}

const (
	ManagerName        = "manager"
	managerHostPort    = 18790
	managerGuestPort   = 18790
	managerDebugMode   = false
	hostPicoClawDir    = ".picoclaw"
	hostPicoClawConfig = "config.json"
	hostPicoClawLogs   = "logs"
	boxPicoClawDir     = "/home/picoclaw/.picoclaw"
)

type Service struct {
	llm          config.LLMConfig
	managerImage string
	state        string
	homeDir      string
	mu           sync.RWMutex
	runtime      *boxlite.Runtime
	boxes        map[string]closer
	agents       map[string]Agent
}

type closer interface {
	Close() error
}

func NewService(llm config.LLMConfig, managerImage, statePath, runtimeHome string) (*Service, error) {
	if managerImage == "" {
		managerImage = config.DefaultManagerImage
	}
	svc := &Service{
		llm:          llm,
		managerImage: managerImage,
		state:        statePath,
		homeDir:      runtimeHome,
		boxes:        make(map[string]closer),
		agents:       make(map[string]Agent),
	}
	if err := svc.load(); err != nil {
		return nil, err
	}
	return svc, nil
}

func EnsureBootstrapState(ctx context.Context, statePath, runtimeHome string, server config.ServerConfig, llm config.LLMConfig, pico config.PicoClawConfig, managerImage string, forceRecreate bool) error {
	svc, err := NewService(llm, managerImage, statePath, runtimeHome)
	if err != nil {
		return err
	}
	defer func() {
		_ = svc.Close()
	}()

	rt, err := svc.ensureRuntime()
	if err != nil {
		return err
	}

	var box *boxlite.Box
	if forceRecreate {
		log.Printf("force recreating bootstrap manager box %q", ManagerName)
		if err := rt.ForceRemove(ctx, ManagerName); err != nil {
			if boxlite.IsNotFound(err) {
				log.Printf("bootstrap manager box %q does not exist yet; continuing", ManagerName)
			} else {
				return fmt.Errorf("force remove bootstrap manager box %q: %w", ManagerName, err)
			}
		} else {
			log.Printf("bootstrap manager box %q removed", ManagerName)
		}
	} else {
		box, err = rt.Get(ctx, ManagerName)
	}
	if box == nil {
		boxOpts, err := managerBoxOptions(server, llm, pico)
		if err != nil {
			return err
		}
		log.Printf("bootstrap manager box %q not found, creating it with image %q", ManagerName, svc.managerImage)
		log.Printf("if the image is not present locally, the first pull may take a while")
		progressDone := make(chan struct{})
		go func() {
			ticker := time.NewTicker(10 * time.Second)
			defer ticker.Stop()
			for {
				select {
				case <-progressDone:
					return
				case <-ticker.C:
					log.Printf("still creating bootstrap manager box %q with image %q; image download may still be in progress", ManagerName, svc.managerImage)
				}
			}
		}()
		box, err = rt.Create(ctx, svc.managerImage, boxOpts...)
		close(progressDone)
		if err != nil {
			return fmt.Errorf("create bootstrap manager box: %w", err)
		}
		log.Printf("bootstrap manager box %q created", ManagerName)
	}

	svc.mu.Lock()
	defer svc.mu.Unlock()

	if err := box.Start(ctx); err != nil {
		return fmt.Errorf("start bootstrap manager box: %w", err)
	}

	info, err := box.Info(ctx)
	if err != nil {
		return fmt.Errorf("read bootstrap manager box info: %w", err)
	}

	manager := Agent{
		ID:        info.ID,
		Name:      ManagerName,
		Image:     svc.managerImage,
		Status:    string(info.State),
		CreatedAt: info.CreatedAt.UTC(),
		ModelID:   llm.ModelID,
	}
	for id, a := range svc.agents {
		if a.Name == ManagerName && id != manager.ID {
			delete(svc.agents, id)
		}
	}
	svc.agents[manager.ID] = manager
	svc.boxes[manager.ID] = box
	return svc.saveLocked()
}

func (s *Service) Create(ctx context.Context, req CreateRequest) (Agent, error) {
	if req.Name == "" {
		return Agent{}, fmt.Errorf("name is required")
	}
	if req.Image == "" {
		return Agent{}, fmt.Errorf("image is required")
	}

	rt, err := s.ensureRuntime()
	if err != nil {
		return Agent{}, err
	}

	id := fmt.Sprintf("agent-%d", time.Now().UnixNano())
	box, err := rt.Create(ctx, req.Image,
		boxlite.WithName(req.Name),
		boxlite.WithAutoRemove(false),
		boxlite.WithEnv("CSGCLAW_LLM_BASE_URL", s.llm.BaseURL),
		boxlite.WithEnv("CSGCLAW_LLM_API_KEY", s.llm.APIKey),
		boxlite.WithEnv("CSGCLAW_LLM_MODEL_ID", s.llm.ModelID),
		boxlite.WithEnv("OPENAI_BASE_URL", s.llm.BaseURL),
		boxlite.WithEnv("OPENAI_API_KEY", s.llm.APIKey),
		boxlite.WithEnv("OPENAI_MODEL", s.llm.ModelID),
	)
	if err != nil {
		return Agent{}, fmt.Errorf("create boxlite agent: %w", err)
	}

	agent := Agent{
		ID:        id,
		Name:      req.Name,
		Image:     req.Image,
		Status:    "running",
		CreatedAt: time.Now().UTC(),
		ModelID:   s.llm.ModelID,
	}

	s.mu.Lock()
	s.boxes[id] = box
	s.agents[id] = agent
	err = s.saveLocked()
	s.mu.Unlock()
	if err != nil {
		return Agent{}, err
	}
	return agent, nil
}

func (s *Service) List() []Agent {
	s.mu.RLock()
	defer s.mu.RUnlock()

	agents := make([]Agent, 0, len(s.agents))
	for _, a := range s.agents {
		agents = append(agents, a)
	}
	slices.SortFunc(agents, func(a, b Agent) int {
		if a.CreatedAt.Equal(b.CreatedAt) {
			switch {
			case a.ID < b.ID:
				return -1
			case a.ID > b.ID:
				return 1
			default:
				return 0
			}
		}
		if a.CreatedAt.Before(b.CreatedAt) {
			return -1
		}
		return 1
	})
	return agents
}

func (s *Service) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	for id, box := range s.boxes {
		_ = box.Close()
		delete(s.boxes, id)
	}
	if s.runtime != nil {
		err := s.runtime.Close()
		s.runtime = nil
		return err
	}
	return nil
}

func managerBoxOptions(server config.ServerConfig, llm config.LLMConfig, pico config.PicoClawConfig) ([]boxlite.BoxOption, error) {
	opts := []boxlite.BoxOption{
		boxlite.WithName(ManagerName),
		boxlite.WithDetach(true),
		boxlite.WithAutoRemove(false),
		//boxlite.WithPort(managerHostPort, managerGuestPort),
		boxlite.WithEnv("HOME", "/home/picoclaw"),
		boxlite.WithEnv("CSGCLAW_LLM_BASE_URL", llm.BaseURL),
		boxlite.WithEnv("CSGCLAW_LLM_API_KEY", llm.APIKey),
		boxlite.WithEnv("CSGCLAW_LLM_MODEL_ID", llm.ModelID),
		boxlite.WithEnv("OPENAI_BASE_URL", llm.BaseURL),
		boxlite.WithEnv("OPENAI_API_KEY", llm.APIKey),
		boxlite.WithEnv("OPENAI_MODEL", llm.ModelID),
	}
	if managerDebugMode {
		opts = append(opts,
			boxlite.WithEntrypoint("sleep"),
			boxlite.WithCmd("infinity"),
		)
	} else {
		opts = append(opts,
			boxlite.WithEntrypoint("picoclaw"),
			boxlite.WithCmd("gateway"),
		)
	}

	hostPicoClawRoot, err := ensureManagerPicoClawConfig(server, llm, pico)
	if err != nil {
		return nil, err
	}
	opts = append(opts, boxlite.WithVolume(hostPicoClawRoot, boxPicoClawDir))

	return opts, nil
}

func (s *Service) load() error {
	if s.state == "" {
		return nil
	}

	data, err := os.ReadFile(s.state)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("read agent state: %w", err)
	}

	var agents []Agent
	if err := json.Unmarshal(data, &agents); err != nil {
		return fmt.Errorf("decode agent state: %w", err)
	}
	for _, a := range agents {
		s.agents[a.ID] = a
	}
	return nil
}

func (s *Service) saveLocked() error {
	if s.state == "" {
		return nil
	}

	agents := make([]Agent, 0, len(s.agents))
	for _, a := range s.agents {
		agents = append(agents, a)
	}
	slices.SortFunc(agents, func(a, b Agent) int {
		if a.CreatedAt.Equal(b.CreatedAt) {
			switch {
			case a.ID < b.ID:
				return -1
			case a.ID > b.ID:
				return 1
			default:
				return 0
			}
		}
		if a.CreatedAt.Before(b.CreatedAt) {
			return -1
		}
		return 1
	})

	data, err := json.MarshalIndent(agents, "", "  ")
	if err != nil {
		return fmt.Errorf("encode agent state: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(s.state), 0o755); err != nil {
		return fmt.Errorf("create state dir: %w", err)
	}
	if err := os.WriteFile(s.state, append(data, '\n'), 0o600); err != nil {
		return fmt.Errorf("write agent state: %w", err)
	}
	return nil
}

func (s *Service) ensureRuntime() (*boxlite.Runtime, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.runtime != nil {
		return s.runtime, nil
	}

	opts := []boxlite.RuntimeOption{}
	if s.homeDir != "" {
		opts = append(opts, boxlite.WithHomeDir(s.homeDir))
	}

	rt, err := boxlite.NewRuntime(opts...)
	if err != nil {
		return nil, fmt.Errorf("create boxlite runtime: %w", err)
	}
	s.runtime = rt
	return s.runtime, nil
}
