package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"time"

	boxlite "github.com/RussellLuo/boxlite/sdks/go"

	"csgclaw/internal/config"
)

const (
	RoleAgent   = "agent"
	RoleWorker  = "worker"
	RoleManager = "manager"
)

type Agent struct {
	ID          string    `json:"id"`
	Name        string    `json:"name"`
	Description string    `json:"description,omitempty"`
	Image       string    `json:"image,omitempty"`
	Role        string    `json:"role"`
	Status      string    `json:"status"`
	CreatedAt   time.Time `json:"created_at"`
	ModelID     string    `json:"model_id,omitempty"`
}

type CreateRequest struct {
	ID          string    `json:"id,omitempty"`
	Name        string    `json:"name"`
	Description string    `json:"description,omitempty"`
	Image       string    `json:"image,omitempty"`
	Role        string    `json:"role,omitempty"`
	Status      string    `json:"status,omitempty"`
	CreatedAt   time.Time `json:"created_at,omitempty"`
	ModelID     string    `json:"model_id,omitempty"`
}

const (
	ManagerName        = "manager"
	ManagerUserID      = "u-manager"
	managerHostPort    = 18790
	managerGuestPort   = 18790
	managerDebugMode   = true
	hostPicoClawDir    = ".picoclaw"
	hostProjectsDir    = "projects"
	hostPicoClawConfig = "config.json"
	hostPicoClawLogs   = "logs"
	boxPicoClawDir     = "/home/picoclaw/.picoclaw"
	boxProjectsDir     = "/home/picoclaw/.picoclaw/workspace/projects"
)

var en0IPv4Resolver = en0IPv4

type Service struct {
	llm          config.LLMConfig
	server       config.ServerConfig
	pico         config.PicoClawConfig
	managerImage string
	state        string
	mu           sync.RWMutex
	runtimes     map[string]*boxlite.Runtime
	boxes        map[string]closer
	agents       map[string]Agent
}

type closer interface {
	Close() error
}

func NewService(llm config.LLMConfig, server config.ServerConfig, pico config.PicoClawConfig, managerImage, statePath, runtimeHome string) (*Service, error) {
	if managerImage == "" {
		managerImage = config.DefaultManagerImage
	}
	svc := &Service{
		llm:          llm,
		server:       server,
		pico:         pico,
		managerImage: managerImage,
		state:        statePath,
		runtimes:     make(map[string]*boxlite.Runtime),
		boxes:        make(map[string]closer),
		agents:       make(map[string]Agent),
	}
	if err := svc.load(); err != nil {
		return nil, err
	}
	return svc, nil
}

func EnsureBootstrapState(ctx context.Context, statePath, runtimeHome string, server config.ServerConfig, llm config.LLMConfig, pico config.PicoClawConfig, managerImage string, forceRecreate bool) error {
	svc, err := NewService(llm, server, pico, managerImage, statePath, runtimeHome)
	if err != nil {
		return err
	}
	defer func() {
		_ = svc.Close()
	}()

	rt, err := svc.ensureRuntime(ManagerName)
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
	var info *boxlite.BoxInfo
	if box == nil {
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
		box, info, err = svc.createGatewayBox(ctx, rt, svc.managerImage, ManagerName, ManagerUserID, llm.ModelID)
		close(progressDone)
		if err != nil {
			return fmt.Errorf("create bootstrap manager box: %w", err)
		}
		log.Printf("bootstrap manager box %q created", ManagerName)
	} else {
		if err := box.Start(ctx); err != nil {
			return fmt.Errorf("start bootstrap manager box: %w", err)
		}
		info, err = box.Info(ctx)
		if err != nil {
			return fmt.Errorf("read bootstrap manager box info: %w", err)
		}
	}

	svc.mu.Lock()
	defer svc.mu.Unlock()

	manager := Agent{
		ID:        ManagerUserID,
		Name:      ManagerName,
		Image:     svc.managerImage,
		Status:    string(info.State),
		CreatedAt: info.CreatedAt.UTC(),
		ModelID:   llm.ModelID,
		Role:      RoleManager,
	}
	for id, a := range svc.agents {
		if isManagerAgent(a) && id != manager.ID {
			delete(svc.agents, id)
		}
	}
	svc.agents[manager.ID] = manager
	svc.boxes[manager.ID] = box
	return svc.saveLocked()
}

func (s *Service) Create(ctx context.Context, req CreateRequest) (Agent, error) {
	id := strings.TrimSpace(req.ID)
	name := strings.TrimSpace(req.Name)
	description := strings.TrimSpace(req.Description)
	image := strings.TrimSpace(req.Image)
	role := normalizeRole(req.Role)
	modelID := strings.TrimSpace(req.ModelID)
	if name == "" {
		return Agent{}, fmt.Errorf("name is required")
	}
	if image == "" {
		return Agent{}, fmt.Errorf("image is required")
	}
	if role == RoleManager {
		return Agent{}, fmt.Errorf("role %q is reserved", role)
	}
	if id == "" {
		id = fmt.Sprintf("%s-%d", role, time.Now().UnixNano())
	}

	s.mu.RLock()
	idExists := false
	if _, ok := s.agents[id]; ok {
		idExists = true
	}
	nameExists := s.hasNameLocked(name)
	s.mu.RUnlock()
	if idExists {
		return Agent{}, fmt.Errorf("agent id %q already exists", id)
	}
	if nameExists {
		return Agent{}, fmt.Errorf("agent name %q already exists", name)
	}

	rt, err := s.ensureRuntime(name)
	if err != nil {
		return Agent{}, err
	}

	if modelID == "" {
		modelID = s.llm.ModelID
	}

	projectsRoot, err := ensureAgentProjectsRoot()
	if err != nil {
		return Agent{}, err
	}
	box, err := rt.Create(ctx, image,
		boxlite.WithName(name),
		boxlite.WithAutoRemove(false),
		boxlite.WithEnv("CSGCLAW_LLM_BASE_URL", s.llm.BaseURL),
		boxlite.WithEnv("CSGCLAW_LLM_API_KEY", s.llm.APIKey),
		boxlite.WithEnv("CSGCLAW_LLM_MODEL_ID", modelID),
		boxlite.WithEnv("OPENAI_BASE_URL", s.llm.BaseURL),
		boxlite.WithEnv("OPENAI_API_KEY", s.llm.APIKey),
		boxlite.WithEnv("OPENAI_MODEL", modelID),
		boxlite.WithVolume(projectsRoot, boxProjectsDir),
	)
	if err != nil {
		return Agent{}, fmt.Errorf("create boxlite agent: %w", err)
	}

	createdAt := req.CreatedAt.UTC()
	if req.CreatedAt.IsZero() {
		createdAt = time.Now().UTC()
	}
	status := strings.TrimSpace(req.Status)
	if status == "" {
		status = "running"
	}
	agent := Agent{
		ID:          id,
		Name:        name,
		Description: description,
		Image:       image,
		Role:        role,
		Status:      status,
		CreatedAt:   createdAt,
		ModelID:     modelID,
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

func (s *Service) Agent(id string) (Agent, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	a, ok := s.agents[strings.TrimSpace(id)]
	if !ok {
		return Agent{}, false
	}
	return *cloneAgent(&a), true
}

func (s *Service) List() []Agent {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return sortedAgentsFromMap(s.agents)
}

func (s *Service) CreateWorker(ctx context.Context, req CreateRequest) (Agent, error) {
	id := strings.TrimSpace(req.ID)
	name := strings.TrimSpace(req.Name)
	description := strings.TrimSpace(req.Description)
	modelID := strings.TrimSpace(req.ModelID)

	switch {
	case name == "":
		return Agent{}, fmt.Errorf("name is required")
	case strings.EqualFold(name, ManagerName):
		return Agent{}, fmt.Errorf("name %q is reserved", name)
	}
	if id == "" {
		// id = fmt.Sprintf("%s-%d", RoleWorker, time.Now().UnixNano())
		id = fmt.Sprintf("u-%s", name)
	}

	s.mu.RLock()
	idExists := false
	if _, ok := s.agents[id]; ok {
		idExists = true
	}
	nameExists := s.hasNameLocked(name)
	s.mu.RUnlock()
	if idExists {
		return Agent{}, fmt.Errorf("agent id %q already exists", id)
	}
	if nameExists {
		return Agent{}, fmt.Errorf("agent name %q already exists", name)
	}

	rt, err := s.ensureRuntime(name)
	if err != nil {
		return Agent{}, err
	}
	if modelID == "" {
		modelID = s.llm.ModelID
	}

	box, info, err := s.createGatewayBox(ctx, rt, s.managerImage, name, id, modelID)
	if err != nil {
		return Agent{}, fmt.Errorf("create worker box: %w", err)
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	if _, ok := s.agents[id]; ok {
		_ = box.Close()
		return Agent{}, fmt.Errorf("agent id %q already exists", id)
	}
	if s.hasNameLocked(name) {
		_ = box.Close()
		return Agent{}, fmt.Errorf("agent name %q already exists", name)
	}

	worker := Agent{
		ID:          id,
		Name:        name,
		Image:       s.managerImage,
		Description: description,
		Status:      string(info.State),
		CreatedAt:   info.CreatedAt.UTC(),
		ModelID:     modelID,
		Role:        RoleWorker,
	}
	s.boxes[worker.ID] = box
	s.agents[worker.ID] = worker
	if err := s.saveLocked(); err != nil {
		delete(s.boxes, worker.ID)
		delete(s.agents, worker.ID)
		_ = box.Close()
		return Agent{}, err
	}
	return *cloneAgent(&worker), nil
}

func (s *Service) createGatewayBox(ctx context.Context, rt *boxlite.Runtime, image, name, botID, modelID string) (*boxlite.Box, *boxlite.BoxInfo, error) {
	boxOpts, err := s.gatewayBoxOptions(name, botID, modelID)
	if err != nil {
		return nil, nil, err
	}
	box, err := rt.Create(ctx, image, boxOpts...)
	if err != nil {
		return nil, nil, fmt.Errorf("create gateway box: %w", err)
	}
	if err := box.Start(ctx); err != nil {
		_ = box.Close()
		return nil, nil, fmt.Errorf("start gateway box: %w", err)
	}
	info, err := box.Info(ctx)
	if err != nil {
		_ = box.Close()
		return nil, nil, fmt.Errorf("read gateway box info: %w", err)
	}
	return box, info, nil
}

func (s *Service) ListWorkers() []Agent {
	s.mu.RLock()
	defer s.mu.RUnlock()

	workers := make(map[string]Agent)
	for id, a := range s.agents {
		if a.Role == RoleWorker {
			workers[id] = a
		}
	}
	return sortedAgentsFromMap(workers)
}

func (s *Service) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	for id, box := range s.boxes {
		_ = box.Close()
		delete(s.boxes, id)
	}
	var closeErr error
	for name, rt := range s.runtimes {
		if err := rt.Close(); err != nil && closeErr == nil {
			closeErr = err
		}
		delete(s.runtimes, name)
	}
	return closeErr
}

func (s *Service) gatewayBoxOptions(name, botID, modelID string) ([]boxlite.BoxOption, error) {
	if strings.TrimSpace(modelID) == "" {
		modelID = s.llm.ModelID
	}
	envVars := picoclawBoxEnvVars(resolveManagerBaseURL(s.server), s.pico.AccessToken, botID, s.llm)
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

	//hostPicoClawRoot, err := ensureAgentPicoClawConfig(name, botID, s.server, s.llm, s.pico)
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

	var state persistedState
	if err := json.Unmarshal(data, &state); err == nil && state.isObject() {
		for _, a := range state.Agents {
			normalized := s.normalizeLoadedAgent(a)
			s.agents[normalized.ID] = normalized
		}
		for _, w := range state.Workers {
			normalized := s.normalizeLoadedAgent(w.toAgent())
			s.agents[normalized.ID] = normalized
		}
		return nil
	}

	var agents []Agent
	if err := json.Unmarshal(data, &agents); err != nil {
		return fmt.Errorf("decode agent state: %w", err)
	}
	for _, a := range agents {
		normalized := s.normalizeLoadedAgent(a)
		s.agents[normalized.ID] = normalized
	}
	return nil
}

func (s *Service) saveLocked() error {
	if s.state == "" {
		return nil
	}

	data, err := json.MarshalIndent(persistedState{
		Agents: sortedAgentsFromMap(s.agents),
	}, "", "  ")
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

type persistedState struct {
	Agents  []Agent        `json:"agents"`
	Workers []legacyWorker `json:"workers,omitempty"`
}

func (s persistedState) isObject() bool {
	return s.Agents != nil || s.Workers != nil
}

type legacyWorker struct {
	ID          string    `json:"id"`
	Name        string    `json:"name"`
	Description string    `json:"description,omitempty"`
	Status      string    `json:"status"`
	CreatedAt   time.Time `json:"created_at"`
	ModelID     string    `json:"model_id,omitempty"`
}

func (w legacyWorker) toAgent() Agent {
	agent := Agent{
		ID:          w.ID,
		Name:        w.Name,
		Description: w.Description,
		Image:       "",
		Role:        RoleWorker,
		Status:      w.Status,
		CreatedAt:   w.CreatedAt,
		ModelID:     w.ModelID,
	}
	return agent
}

func (s *Service) normalizeLoadedAgent(a Agent) Agent {
	a = *cloneAgent(&a)
	a.Role = normalizeRole(a.Role)
	if isManagerAgent(a) {
		a.ID = ManagerUserID
		a.Name = ManagerName
		a.Role = RoleManager
		if strings.TrimSpace(a.Image) == "" {
			a.Image = s.managerImage
		}
	}
	return a
}

func normalizeRole(role string) string {
	switch strings.ToLower(strings.TrimSpace(role)) {
	case "", RoleAgent:
		return RoleAgent
	case RoleWorker:
		return RoleWorker
	case RoleManager:
		return RoleManager
	default:
		return strings.ToLower(strings.TrimSpace(role))
	}
}

func isManagerAgent(a Agent) bool {
	return strings.EqualFold(strings.TrimSpace(a.Role), RoleManager) ||
		strings.EqualFold(strings.TrimSpace(a.Name), ManagerName) ||
		strings.EqualFold(strings.TrimSpace(a.ID), ManagerUserID)
}

func sortedAgentsFromMap(items map[string]Agent) []Agent {
	agents := make([]Agent, 0, len(items))
	for _, a := range items {
		agents = append(agents, *cloneAgent(&a))
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

func (s *Service) hasNameLocked(name string) bool {
	for _, existing := range s.agents {
		if strings.EqualFold(existing.Name, name) {
			return true
		}
	}
	return false
}

func cloneAgent(src *Agent) *Agent {
	if src == nil {
		return nil
	}
	dst := *src
	return &dst
}

func (s *Service) ensureRuntime(agentName string) (*boxlite.Runtime, error) {
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
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("resolve host home dir: %w", err)
	}
	return filepath.Join(homeDir, config.AppDirName, managerAgentsDirName, agentName, config.RuntimeHomeDirName), nil
}
