package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"csgclaw/internal/agent"
	"csgclaw/internal/config"
	"csgclaw/internal/im"
	"csgclaw/internal/server"
)

func main() {
	log.SetFlags(0)

	if len(os.Args) < 2 {
		usage()
		os.Exit(2)
	}

	ctx := context.Background()
	switch os.Args[1] {
	case "onboard":
		if err := runOnboard(); err != nil {
			log.Fatal(err)
		}
	case "start":
		if err := runStart(); err != nil {
			log.Fatal(err)
		}
	case "_serve":
		if err := runServe(ctx); err != nil {
			log.Fatal(err)
		}
	case "create":
		if err := runCreate(ctx); err != nil {
			log.Fatal(err)
		}
	case "list":
		if err := runList(ctx); err != nil {
			log.Fatal(err)
		}
	default:
		usage()
		os.Exit(2)
	}
}

func usage() {
	fmt.Fprintf(os.Stderr, `csgclaw manages local CSGClaw agents.

Usage:
  csgclaw onboard [--force-recreate-manager]
  csgclaw start [-d|--daemon]
  csgclaw create --name NAME --image IMAGE
  csgclaw list
`)
}

func runOnboard() error {
	fs := flag.NewFlagSet("onboard", flag.ContinueOnError)
	baseURL := fs.String("base-url", config.DefaultLLMBaseURL, "LLM provider base URL")
	apiKey := fs.String("api-key", config.DefaultLLMAPIKey, "LLM provider API key")
	modelID := fs.String("model-id", config.DefaultLLMModelID, "LLM model identifier")
	managerImage := fs.String("manager-image", config.DefaultManagerImage, "bootstrap manager image")
	forceRecreateManager := fs.Bool("force-recreate-manager", false, "remove and recreate the bootstrap manager box")
	if err := fs.Parse(os.Args[2:]); err != nil {
		return err
	}

	cfg := config.Config{
		Server: config.ServerConfig{
			ListenAddr: config.DefaultListenAddr,
			APIBaseURL: config.DefaultAPIBaseURL,
		},
		LLM: config.LLMConfig{
			BaseURL: *baseURL,
			APIKey:  *apiKey,
			ModelID: *modelID,
		},
		Bootstrap: config.BootstrapConfig{
			ManagerImage: *managerImage,
		},
		PicoClaw: config.PicoClawConfig{
			AccessToken: config.DefaultPicoClawAccessToken,
		},
	}

	path, err := config.DefaultPath()
	if err != nil {
		return err
	}
	if err := cfg.Save(path); err != nil {
		return err
	}

	agentsPath, err := config.DefaultAgentsPath()
	if err != nil {
		return err
	}
	runtimeHome, err := config.DefaultRuntimeHome()
	if err != nil {
		return err
	}
	if err := agent.EnsureBootstrapState(context.Background(), agentsPath, runtimeHome, cfg.Server, cfg.LLM, cfg.PicoClaw, cfg.Bootstrap.ManagerImage, *forceRecreateManager); err != nil {
		return err
	}
	imStatePath, err := config.DefaultIMStatePath()
	if err != nil {
		return err
	}
	if err := im.EnsureBootstrapState(imStatePath); err != nil {
		return err
	}

	fmt.Printf("initialized config at %s\n", path)
	fmt.Printf("ensured bootstrap agent %q with image %q\n", agent.ManagerName, cfg.Bootstrap.ManagerImage)
	fmt.Printf("ensured IM members %q and %q\n", "Admin", "Manager")
	fmt.Println("cleared IM invite draft data")
	if *forceRecreateManager {
		fmt.Println("manager box was force-recreated")
	}
	return nil
}

func runStart() error {
	fs := flag.NewFlagSet("start", flag.ContinueOnError)
	daemon := fs.Bool("daemon", false, "run server in background")
	fs.BoolVar(daemon, "d", false, "run server in background")
	if err := fs.Parse(os.Args[2:]); err != nil {
		return err
	}

	cfg, err := config.LoadDefault()
	if err != nil {
		return err
	}

	if *daemon {
		return serveBackground(cfg)
	}

	return serveForeground(cfg)
}

func serveForeground(cfg config.Config) error {
	svc, err := newAgentService(cfg)
	if err != nil {
		return err
	}
	imSvc, err := newIMService()
	if err != nil {
		return err
	}
	imBus := im.NewBus()

	return server.Run(server.Options{
		ListenAddr: cfg.Server.ListenAddr,
		Service:    svc,
		IM:         imSvc,
		IMBus:      imBus,
		PicoClaw:   im.NewPicoClawBridge(cfg.PicoClaw),
	})
}

func serveBackground(cfg config.Config) error {
	exe, err := os.Executable()
	if err != nil {
		return fmt.Errorf("resolve executable: %w", err)
	}

	stateDir, err := config.DefaultDir()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(stateDir, 0o755); err != nil {
		return fmt.Errorf("create state dir: %w", err)
	}

	logPath := filepath.Join(stateDir, "server.log")
	logFile, err := os.OpenFile(logPath, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return fmt.Errorf("open log file: %w", err)
	}
	defer logFile.Close()

	cmd := exec.Command(exe, "_serve")
	cmd.Stdout = logFile
	cmd.Stderr = logFile
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}

	if err := cmd.Start(); err != nil {
		return fmt.Errorf("start daemon: %w", err)
	}

	if err := waitForHealthy(cfg.Server.APIBaseURL, 5*time.Second); err != nil {
		return fmt.Errorf("server process started (pid %d) but health check failed: %w; see %s", cmd.Process.Pid, err, logPath)
	}

	fmt.Printf("server started in background (pid %d)\n", cmd.Process.Pid)
	fmt.Printf("api: %s\n", cfg.Server.APIBaseURL)
	fmt.Printf("log: %s\n", logPath)
	return nil
}

func runServe(ctx context.Context) error {
	ctx, stop := signal.NotifyContext(ctx, os.Interrupt, syscall.SIGTERM)
	defer stop()

	cfg, err := config.LoadDefault()
	if err != nil {
		var pathErr *os.PathError
		if strings.Contains(err.Error(), "run `csgclaw onboard` first") || errors.As(err, &pathErr) {
			cfg = config.Config{
				Server: config.ServerConfig{
					ListenAddr: config.DefaultListenAddr,
					APIBaseURL: config.DefaultAPIBaseURL,
				},
			}
		} else {
			return err
		}
	}

	var svc *agent.Service
	if cfg.LLM != (config.LLMConfig{}) {
		svc, err = newAgentService(cfg)
		if err != nil {
			return err
		}
	}
	imSvc, err := newIMService()
	if err != nil {
		return err
	}
	imBus := im.NewBus()

	return server.Run(server.Options{
		ListenAddr: cfg.Server.ListenAddr,
		Service:    svc,
		IM:         imSvc,
		IMBus:      imBus,
		PicoClaw:   im.NewPicoClawBridge(cfg.PicoClaw),
		Context:    ctx,
	})
}

func runCreate(ctx context.Context) error {
	fs := flag.NewFlagSet("create", flag.ContinueOnError)
	name := fs.String("name", "", "agent name")
	image := fs.String("image", "", "container image")
	if err := fs.Parse(os.Args[2:]); err != nil {
		return err
	}
	if strings.TrimSpace(*name) == "" {
		return errors.New("missing --name")
	}
	if strings.TrimSpace(*image) == "" {
		return errors.New("missing --image")
	}

	cfg, err := config.LoadDefault()
	if err != nil {
		return err
	}

	client := server.NewClient(cfg.Server.APIBaseURL, &http.Client{Timeout: 15 * time.Second})
	resp, err := client.CreateAgent(ctx, server.CreateAgentRequest{
		Name:  *name,
		Image: *image,
	})
	if err != nil {
		return err
	}

	fmt.Printf("agent created\nid: %s\nname: %s\nimage: %s\nstatus: %s\n", resp.ID, resp.Name, resp.Image, resp.Status)
	return nil
}

func runList(ctx context.Context) error {
	cfg, err := config.LoadDefault()
	if err != nil {
		return err
	}

	client := server.NewClient(cfg.Server.APIBaseURL, &http.Client{Timeout: 15 * time.Second})
	agents, err := client.ListAgents(ctx)
	if err != nil {
		return err
	}

	if len(agents) == 0 {
		fmt.Println("no agents")
		return nil
	}

	fmt.Printf("%-28s %-16s %-10s %-20s %s\n", "ID", "NAME", "STATUS", "CREATED_AT", "IMAGE")
	for _, a := range agents {
		fmt.Printf("%-28s %-16s %-10s %-20s %s\n",
			a.ID,
			truncate(a.Name, 16),
			a.Status,
			a.CreatedAt.Local().Format("2006-01-02 15:04:05"),
			a.Image,
		)
	}
	return nil
}

func waitForHealthy(apiBaseURL string, timeout time.Duration) error {
	client := &http.Client{Timeout: 1 * time.Second}
	deadline := time.Now().Add(timeout)
	url := strings.TrimRight(apiBaseURL, "/") + "/healthz"

	var lastErr error
	for time.Now().Before(deadline) {
		resp, err := client.Get(url)
		if err == nil && resp.StatusCode == http.StatusOK {
			_ = resp.Body.Close()
			return nil
		}
		if err == nil {
			lastErr = fmt.Errorf("status %s", resp.Status)
			_ = resp.Body.Close()
		} else {
			lastErr = err
		}
		time.Sleep(200 * time.Millisecond)
	}

	if lastErr == nil {
		lastErr = errors.New("timed out")
	}
	return lastErr
}

func newAgentService(cfg config.Config) (*agent.Service, error) {
	agentsPath, err := config.DefaultAgentsPath()
	if err != nil {
		return nil, err
	}
	runtimeHome, err := config.DefaultRuntimeHome()
	if err != nil {
		return nil, err
	}
	return agent.NewService(cfg.LLM, cfg.Bootstrap.ManagerImage, agentsPath, runtimeHome)
}

func newIMService() (*im.Service, error) {
	imStatePath, err := config.DefaultIMStatePath()
	if err != nil {
		return nil, err
	}
	return im.NewServiceFromPath(imStatePath)
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	if n <= 3 {
		return s[:n]
	}
	return s[:n-3] + "..."
}
