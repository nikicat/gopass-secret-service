package main

import (
	"flag"
	"fmt"
	"log"
	"os"

	"gopkg.in/yaml.v3"

	"github.com/nikicat/gopass-secret-service/internal/config"
)

func runConfig(args []string) {
	if len(args) < 1 {
		fmt.Fprintf(os.Stderr, "Usage: gopass-secret config <show|path> [options]\n")
		os.Exit(1)
	}

	switch args[0] {
	case "show":
		runConfigShow(args[1:])
	case "path":
		runConfigPath(args[1:])
	default:
		fmt.Fprintf(os.Stderr, "Unknown config subcommand: %s\n", args[0])
		fmt.Fprintf(os.Stderr, "Usage: gopass-secret config <show|path> [options]\n")
		os.Exit(1)
	}
}

func runConfigShow(args []string) {
	fs := flag.NewFlagSet("config show", flag.ExitOnError)
	var flags commonFlags
	addCommonFlags(fs, &flags)
	mustParse(fs, args)

	cfg, err := flags.loadConfig()
	if err != nil {
		log.Fatalf("Failed to load config: %v", err)
	}

	// Validate and warn
	if _, err := os.Stat(cfg.StorePath); os.IsNotExist(err) {
		fmt.Fprintf(os.Stderr, "Warning: store path does not exist: %s\n", cfg.StorePath)
	}
	switch cfg.LogLevel {
	case "debug", "info", "warn", "error":
		// valid
	default:
		fmt.Fprintf(os.Stderr, "Warning: invalid log level: %s\n", cfg.LogLevel)
	}
	if cfg.Prefix == "" {
		fmt.Fprintf(os.Stderr, "Warning: prefix is empty\n")
	}

	out, err := yaml.Marshal(cfg)
	if err != nil {
		log.Fatalf("Failed to marshal config: %v", err)
	}
	fmt.Print(string(out))
}

func runConfigPath(args []string) {
	fs := flag.NewFlagSet("config path", flag.ExitOnError)
	var flags commonFlags
	addCommonFlags(fs, &flags)
	mustParse(fs, args)

	resolvedPath := config.ResolveConfigPath(flags.configPath)
	fmt.Println(resolvedPath)
}
