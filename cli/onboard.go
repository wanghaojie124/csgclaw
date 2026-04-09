package cli

import (
	"context"
	"fmt"

	"csgclaw/internal/agent"
	"csgclaw/internal/config"
	"csgclaw/internal/im"
)

func (a *App) runOnboard(args []string, globals GlobalOptions) error {
	fs := a.newCommandFlagSet("onboard", "csgclaw onboard [flags]", "Initialize local config and bootstrap state.")
	baseURL := fs.String("base-url", config.DefaultLLMBaseURL, "LLM provider base URL")
	apiKey := fs.String("api-key", config.DefaultLLMAPIKey, "LLM provider API key")
	modelID := fs.String("model-id", config.DefaultLLMModelID, "LLM model identifier")
	managerImage := fs.String("manager-image", config.DefaultManagerImage, "bootstrap manager image")
	forceRecreateManager := fs.Bool("force-recreate-manager", false, "remove and recreate the bootstrap manager box")
	if err := fs.Parse(args); err != nil {
		return err
	}

	cfg := config.Config{
		Server: config.ServerConfig{
			ListenAddr:  config.DefaultListenAddr,
			AccessToken: config.DefaultAccessToken,
		},
		LLM: config.LLMConfig{
			BaseURL: *baseURL,
			APIKey:  *apiKey,
			ModelID: *modelID,
		},
		Bootstrap: config.BootstrapConfig{ManagerImage: *managerImage},
	}

	path, err := configPath(globals.Config)
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
	if err := agent.EnsureBootstrapState(context.Background(), agentsPath, cfg.Server, cfg.LLM, cfg.Bootstrap.ManagerImage, *forceRecreateManager); err != nil {
		return err
	}

	imStatePath, err := config.DefaultIMStatePath()
	if err != nil {
		return err
	}
	if err := im.EnsureBootstrapState(imStatePath); err != nil {
		return err
	}

	fmt.Fprintf(a.stdout, "initialized config at %s\n", path)
	fmt.Fprintf(a.stdout, "ensured bootstrap agent %q with image %q\n", agent.ManagerName, cfg.Bootstrap.ManagerImage)
	fmt.Fprintf(a.stdout, "ensured IM members %q and %q\n", "admin", "manager")
	fmt.Fprintln(a.stdout, "cleared IM invite draft data")
	if *forceRecreateManager {
		fmt.Fprintln(a.stdout, "manager box was force-recreated")
	}
	return nil
}
