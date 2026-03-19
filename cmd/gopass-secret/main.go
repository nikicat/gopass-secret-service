package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/nikicat/gopass-secret-service/internal/config"
)

// Version is set at build time
var Version = "dev"

type commonFlags struct {
	configPath string
	storePath  string
	prefix     string
	verbose    bool
	debug      bool
}

func addCommonFlags(fs *flag.FlagSet, f *commonFlags) {
	fs.StringVar(&f.configPath, "c", "", "Path to config file")
	fs.StringVar(&f.configPath, "config", "", "Path to config file")
	fs.StringVar(&f.storePath, "s", "", "GoPass store path")
	fs.StringVar(&f.storePath, "store-path", "", "GoPass store path")
	fs.StringVar(&f.prefix, "p", "", "Prefix for secret-service entries")
	fs.StringVar(&f.prefix, "prefix", "", "Prefix for secret-service entries")
	fs.BoolVar(&f.verbose, "v", false, "Enable verbose logging")
	fs.BoolVar(&f.verbose, "verbose", false, "Enable verbose logging")
	fs.BoolVar(&f.debug, "d", false, "Enable debug logging")
	fs.BoolVar(&f.debug, "debug", false, "Enable debug logging")
}

func mustParse(fs *flag.FlagSet, args []string) {
	if err := fs.Parse(args); err != nil {
		os.Exit(1)
	}
}

func (f *commonFlags) loadConfig() (*config.Config, error) {
	resolvedPath := config.ResolveConfigPath(f.configPath)
	cfg, err := config.LoadFromFileAndEnv(resolvedPath)
	if err != nil {
		return nil, err
	}
	if f.storePath != "" {
		cfg.StorePath = f.storePath
	}
	if f.prefix != "" {
		cfg.Prefix = f.prefix
	}
	if f.debug {
		cfg.LogLevel = "debug"
	} else if f.verbose {
		cfg.LogLevel = "info"
	}
	return cfg, nil
}

func main() {
	if len(os.Args) < 2 {
		printUsage()
		os.Exit(1)
	}

	switch os.Args[1] {
	case "service":
		runService(os.Args[2:])
	case "config":
		runConfig(os.Args[2:])
	case "add":
		runAdd(os.Args[2:])
	case "get":
		runGet(os.Args[2:])
	case "list", "ls":
		runList(os.Args[2:])
	case "version", "--version":
		fmt.Printf("gopass-secret version %s\n", Version)
	case "help", "-h", "--help":
		printUsage()
	default:
		fmt.Fprintf(os.Stderr, "Unknown command: %s\n\n", os.Args[1])
		printUsage()
		os.Exit(1)
	}
}

func printUsage() {
	fmt.Print(`gopass-secret - GoPass Secret Service toolkit

Usage:
  gopass-secret <command> [options]

Commands:
  service        Run the D-Bus Secret Service daemon
  config         Show configuration
  add            Add a secret to the store
  get            Look up a secret by type and attributes
  list, ls       List secrets with attributes
  version        Print version
  help           Show this help

Run 'gopass-secret <command> -h' for command-specific help.

Global flags (available for all commands):
  -c, --config PATH        Path to config file
  -s, --store-path PATH    GoPass store path
  -p, --prefix PREFIX      Prefix for secret-service entries
  -v, --verbose            Enable verbose logging
  -d, --debug              Enable debug logging
`)
}
