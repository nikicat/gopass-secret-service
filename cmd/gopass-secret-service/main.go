package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"syscall"

	"github.com/nikicat/gopass-secret-service/internal/config"
	"github.com/nikicat/gopass-secret-service/internal/service"
)

// Version is set at build time
var Version = "dev"

const unitName = "gopass-secret-service.service"

const unitTemplate = `[Unit]
Description=GoPass Secret Service - D-Bus Secret Service backed by GoPass

[Service]
Type=simple
ExecStart=%s
Restart=on-failure
RestartSec=3

[Install]
WantedBy=default.target
`

func main() {
	// Check for subcommands before parsing flags.
	if len(os.Args) > 1 {
		switch os.Args[1] {
		case "install":
			if err := runInstall(); err != nil {
				log.Fatalf("Install failed: %v", err)
			}
			return
		case "uninstall":
			if err := runUninstall(); err != nil {
				log.Fatalf("Uninstall failed: %v", err)
			}
			return
		}
	}

	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("Failed to load config: %v", err)
	}

	if cfg.ShowVersion {
		fmt.Printf("gopass-secret-service version %s\n", Version)
		os.Exit(0)
	}

	// Set up logging
	if cfg.LogFile != "" {
		f, err := os.OpenFile(cfg.LogFile, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
		if err != nil {
			log.Fatalf("Failed to open log file: %v", err)
		}
		defer f.Close()
		log.SetOutput(f)
	}

	log.Printf("Starting gopass-secret-service version %s", Version)
	log.Printf("Using gopass prefix: %s", cfg.Prefix)
	log.Printf("Default collection: %s", cfg.DefaultCollection)

	// Create and start the service
	ctx := context.Background()
	svc, err := service.New(ctx, cfg)
	if err != nil {
		log.Fatalf("Failed to create service: %v", err)
	}

	if err := svc.Start(); err != nil {
		log.Fatalf("Failed to start service: %v", err)
	}

	log.Println("Service started successfully")

	// Wait for shutdown signal
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	sig := <-sigChan
	log.Printf("Received signal %v, shutting down...", sig)

	if err := svc.Stop(); err != nil {
		log.Printf("Error during shutdown: %v", err)
	}

	log.Println("Service stopped")
}

func unitDir() (string, error) {
	configHome := os.Getenv("XDG_CONFIG_HOME")
	if configHome == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", fmt.Errorf("get home dir: %w", err)
		}
		configHome = filepath.Join(home, ".config")
	}
	return filepath.Join(configHome, "systemd", "user"), nil
}

func systemctl(args ...string) error {
	cmd := exec.Command("systemctl", append([]string{"--user"}, args...)...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

func runInstall() error {
	self, err := os.Executable()
	if err != nil {
		return fmt.Errorf("find executable: %w", err)
	}
	self, err = filepath.EvalSymlinks(self)
	if err != nil {
		return fmt.Errorf("resolve executable: %w", err)
	}

	dir, err := unitDir()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("create unit dir: %w", err)
	}

	unitPath := filepath.Join(dir, unitName)
	content := fmt.Sprintf(unitTemplate, self)
	if err := os.WriteFile(unitPath, []byte(content), 0644); err != nil {
		return fmt.Errorf("write unit file: %w", err)
	}
	fmt.Printf("Wrote %s\n", unitPath)

	if err := systemctl("daemon-reload"); err != nil {
		return fmt.Errorf("daemon-reload: %w", err)
	}
	if err := systemctl("enable", unitName); err != nil {
		return fmt.Errorf("enable: %w", err)
	}
	fmt.Printf("Enabled %s\n", unitName)
	fmt.Printf("Start with: systemctl --user start %s\n", unitName)
	return nil
}

func runUninstall() error {
	_ = systemctl("stop", unitName)
	_ = systemctl("disable", unitName)

	dir, err := unitDir()
	if err != nil {
		return err
	}

	unitPath := filepath.Join(dir, unitName)
	if err := os.Remove(unitPath); err != nil {
		if !os.IsNotExist(err) {
			return fmt.Errorf("remove %s: %w", unitName, err)
		}
	} else {
		fmt.Printf("Removed %s\n", unitPath)
	}

	return systemctl("daemon-reload")
}
