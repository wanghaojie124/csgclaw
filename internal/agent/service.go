package agent

import (
	"context"
	"fmt"
	"log"
	"os"
	"strings"
	"sync"
	"time"

	boxlite "github.com/RussellLuo/boxlite/sdks/go"

	"csgclaw/internal/config"
)

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

var (
	testEnsureRuntimeHook    func(*Service, string) (*boxlite.Runtime, error)
	testCreateGatewayBoxHook func(*Service, context.Context, *boxlite.Runtime, string, string, string, string) (*boxlite.Box, *boxlite.BoxInfo, error)
	testForceRemoveBoxHook   func(*Service, context.Context, *boxlite.Runtime, string) error
)

// SetTestHooks installs lightweight hooks for tests that need to bypass runtime/box creation.
func SetTestHooks(
	ensureRuntime func(*Service, string) (*boxlite.Runtime, error),
	createGatewayBox func(*Service, context.Context, *boxlite.Runtime, string, string, string, string) (*boxlite.Box, *boxlite.BoxInfo, error),
) {
	testEnsureRuntimeHook = ensureRuntime
	testCreateGatewayBoxHook = createGatewayBox
}

// ResetTestHooks clears hooks installed via SetTestHooks.
func ResetTestHooks() {
	testEnsureRuntimeHook = nil
	testCreateGatewayBoxHook = nil
	testForceRemoveBoxHook = nil
}

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
		BoxID:     info.ID,
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

func (s *Service) Delete(ctx context.Context, id string) error {
	id = strings.TrimSpace(id)
	if id == "" {
		return fmt.Errorf("agent id is required")
	}

	s.mu.RLock()
	existing, ok := s.agents[id]
	box := s.boxes[id]
	s.mu.RUnlock()
	if !ok {
		return fmt.Errorf("agent %q not found", id)
	}
	if isManagerAgent(existing) {
		return fmt.Errorf("agent %q is reserved", id)
	}

	rt, err := s.ensureRuntime(existing.Name)
	if err != nil {
		return err
	}
	if box != nil {
		_ = box.Close()
	}
	boxIDOrName := strings.TrimSpace(existing.BoxID)
	if boxIDOrName == "" {
		boxIDOrName = existing.Name
	}
	if rt != nil {
		if err := s.forceRemoveBox(ctx, rt, boxIDOrName); err != nil && !boxlite.IsNotFound(err) {
			return fmt.Errorf("remove agent box: %w", err)
		}
		_ = rt.Close()
	}

	agentHome, err := agentHomeDir(existing.Name)
	if err != nil {
		return err
	}
	if err := os.RemoveAll(agentHome); err != nil {
		return fmt.Errorf("remove agent home: %w", err)
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	current, ok := s.agents[id]
	if !ok {
		return fmt.Errorf("agent %q not found", id)
	}
	delete(s.agents, id)
	delete(s.boxes, id)
	if rt := s.runtimes[current.Name]; rt != nil {
		delete(s.runtimes, current.Name)
	}
	return s.saveLocked()
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
		BoxID:       info.ID,
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

func (s *Service) hasNameLocked(name string) bool {
	for _, existing := range s.agents {
		if strings.EqualFold(existing.Name, name) {
			return true
		}
	}
	return false
}
