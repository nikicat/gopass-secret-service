package config

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

// Config holds the configuration for gopass-secret-service
type Config struct {
	// StorePath is the path to the GoPass store
	StorePath string `yaml:"store_path"`

	// Prefix is the prefix for secret-service entries in gopass
	Prefix string `yaml:"prefix"`

	// DefaultCollection is the name of the default collection
	DefaultCollection string `yaml:"default_collection"`

	// LogLevel is the logging level (debug, info, warn, error)
	LogLevel string `yaml:"log_level"`

	// LogFile is the path to the log file (empty for stderr)
	LogFile string `yaml:"log_file"`

	// Replace indicates whether to replace an existing secret-service provider
	Replace bool `yaml:"replace"`

	// Verbose enables verbose logging
	Verbose bool `yaml:"-"`

	// Debug enables debug logging
	Debug bool `yaml:"-"`

	// ConfigPath is the path to the config file (set via CLI)
	ConfigPath string `yaml:"-"`

	// ShowVersion indicates whether to print version and exit
	ShowVersion bool `yaml:"-"`
}

// DefaultConfig returns a new Config with default values
func DefaultConfig() *Config {
	homeDir, _ := os.UserHomeDir()
	return &Config{
		StorePath:         filepath.Join(homeDir, ".local/share/gopass/stores/root"),
		Prefix:            "secret-service",
		DefaultCollection: "default",
		LogLevel:          "info",
		LogFile:           "",
		Replace:           false,
	}
}

// Load loads configuration from CLI flags, environment, and config file
func Load() (*Config, error) {
	cfg := DefaultConfig()

	// Parse CLI flags first to get config path
	configPath := flag.String("c", "", "Path to config file")
	flag.StringVar(configPath, "config", "", "Path to config file")
	storePath := flag.String("s", "", "GoPass store path")
	flag.StringVar(storePath, "store-path", "", "GoPass store path")
	prefix := flag.String("p", "", "Prefix for secret-service entries in gopass")
	flag.StringVar(prefix, "prefix", "", "Prefix for secret-service entries in gopass")
	verbose := flag.Bool("v", false, "Enable verbose logging")
	flag.BoolVar(verbose, "verbose", false, "Enable verbose logging")
	debug := flag.Bool("d", false, "Enable debug logging")
	flag.BoolVar(debug, "debug", false, "Enable debug logging")
	replace := flag.Bool("r", false, "Replace existing secret-service provider")
	flag.BoolVar(replace, "replace", false, "Replace existing secret-service provider")
	version := flag.Bool("version", false, "Print version and exit")
	help := flag.Bool("h", false, "Show help message")
	flag.BoolVar(help, "help", false, "Show help message")

	flag.Parse()

	if *help {
		printUsage()
		os.Exit(0)
	}

	cfg.ShowVersion = *version
	cfg.Verbose = *verbose
	cfg.Debug = *debug
	if *replace {
		cfg.Replace = true
	}

	// Determine config file path
	if *configPath != "" {
		cfg.ConfigPath = *configPath
	} else if envPath := os.Getenv("GOPASS_SECRET_SERVICE_CONFIG"); envPath != "" {
		cfg.ConfigPath = envPath
	} else {
		homeDir, _ := os.UserHomeDir()
		cfg.ConfigPath = filepath.Join(homeDir, ".config/gopass-secret-service/config.yaml")
	}

	// Load config file if it exists
	if err := cfg.loadFromFile(); err != nil {
		// Only error if the file exists but can't be read
		if !os.IsNotExist(err) {
			return nil, fmt.Errorf("loading config file: %w", err)
		}
	}

	// Apply environment variables (override config file)
	cfg.applyEnv()

	// Apply CLI flags (override everything)
	if *storePath != "" {
		cfg.StorePath = *storePath
	}
	if *prefix != "" {
		cfg.Prefix = *prefix
	}

	// Expand ~ in paths
	cfg.StorePath = expandPath(cfg.StorePath)
	cfg.LogFile = expandPath(cfg.LogFile)

	// Set log level based on flags
	if cfg.Debug {
		cfg.LogLevel = "debug"
	} else if cfg.Verbose {
		cfg.LogLevel = "info"
	}

	return cfg, nil
}

func (c *Config) loadFromFile() error {
	data, err := os.ReadFile(c.ConfigPath)
	if err != nil {
		return err
	}
	return yaml.Unmarshal(data, c)
}

func (c *Config) applyEnv() {
	if v := os.Getenv("GOPASS_SECRET_SERVICE_STORE_PATH"); v != "" {
		c.StorePath = v
	}
	if v := os.Getenv("GOPASS_SECRET_SERVICE_PREFIX"); v != "" {
		c.Prefix = v
	}
	if v := os.Getenv("GOPASS_SECRET_SERVICE_DEFAULT_COLLECTION"); v != "" {
		c.DefaultCollection = v
	}
	if v := os.Getenv("GOPASS_SECRET_SERVICE_LOG_LEVEL"); v != "" {
		c.LogLevel = v
	}
	if v := os.Getenv("GOPASS_SECRET_SERVICE_LOG_FILE"); v != "" {
		c.LogFile = v
	}
	if v := os.Getenv("GOPASS_SECRET_SERVICE_REPLACE"); v == "true" || v == "1" {
		c.Replace = true
	}
}

func expandPath(path string) string {
	if path == "" {
		return path
	}
	if path[0] == '~' {
		homeDir, _ := os.UserHomeDir()
		return filepath.Join(homeDir, path[1:])
	}
	return path
}

func printUsage() {
	fmt.Println(`gopass-secret-service - D-Bus Secret Service backed by GoPass

Usage:
  gopass-secret-service [options]

Options:
  -c, --config PATH        Path to config file (default: ~/.config/gopass-secret-service/config.yaml)
  -s, --store-path PATH    GoPass store path (default: ~/.local/share/gopass/stores/root)
  -p, --prefix PREFIX      Prefix for secret-service entries in gopass (default: "secret-service")
  -v, --verbose            Enable verbose logging
  -d, --debug              Enable debug logging
  -r, --replace            Replace existing secret-service provider
      --version            Print version and exit
  -h, --help               Show help message

Environment variables:
  GOPASS_SECRET_SERVICE_CONFIG             Path to config file
  GOPASS_SECRET_SERVICE_STORE_PATH         GoPass store path
  GOPASS_SECRET_SERVICE_PREFIX             Prefix for secret-service entries
  GOPASS_SECRET_SERVICE_DEFAULT_COLLECTION Default collection name
  GOPASS_SECRET_SERVICE_LOG_LEVEL          Log level (debug, info, warn, error)
  GOPASS_SECRET_SERVICE_LOG_FILE           Log file path
  GOPASS_SECRET_SERVICE_REPLACE            Replace existing provider (true/1)`)
}
