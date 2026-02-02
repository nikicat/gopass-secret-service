package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/nikicat/gopass-secret-service/internal/config"
	"github.com/nikicat/gopass-secret-service/internal/service"
)

// Version is set at build time
var Version = "dev"

func main() {
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
