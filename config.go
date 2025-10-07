package godevwatch

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

// BuildRule represents a conditional build rule
type BuildRule struct {
	Name    string   `yaml:"name"`
	Watch   []string `yaml:"watch"`
	Command string   `yaml:"command"`
}

// Config represents the configuration for the dev server
type Config struct {
	ProxyPort      int         `yaml:"proxy_port"`
	BackendPort    int         `yaml:"backend_port"`
	BuildStatusDir string      `yaml:"build_status_dir"`
	BuildRules     []BuildRule `yaml:"build_rules"`
	RunCmd         string      `yaml:"run_cmd"`
	InjectScript   bool        `yaml:"inject_script"`
}

// DefaultConfig returns a default configuration
func DefaultConfig() *Config {
	return &Config{
		ProxyPort:      3000,
		BackendPort:    8080,
		BuildStatusDir: "tmp/.build-status",
		BuildRules: []BuildRule{
			{
				Name:    "templ",
				Watch:   []string{"**/*.templ"},
				Command: "templ generate",
			},
			{
				Name:    "go-build",
				Watch:   []string{"**/*.go"},
				Command: "go build -o ./tmp/main .",
			},
		},
		RunCmd:       "./tmp/main",
		InjectScript: true,
	}
}

// LoadConfig loads configuration from a YAML file
func LoadConfig(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read config file: %w", err)
	}

	config := DefaultConfig()
	if err := yaml.Unmarshal(data, config); err != nil {
		return nil, fmt.Errorf("failed to parse config file: %w", err)
	}

	return config, nil
}

// Save saves the configuration to a YAML file
func (c *Config) Save(path string) error {
	data, err := yaml.Marshal(c)
	if err != nil {
		return fmt.Errorf("failed to marshal config: %w", err)
	}

	if err := os.WriteFile(path, data, 0644); err != nil {
		return fmt.Errorf("failed to write config file: %w", err)
	}

	return nil
}
