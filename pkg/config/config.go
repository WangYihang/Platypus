package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/BurntSushi/toml"
	"github.com/WangYihang/Platypus/pkg/listeners"
	"gopkg.in/yaml.v2"
)

// Config represents the configuration.
type Config struct {
	PlainListeners     []listeners.PlainListener     `json:"plain" yaml:"plain" toml:"plain"`
	EncryptedListeners []listeners.EncryptedListener `json:"encrypted" yaml:"encrypted" toml:"encrypted"`
	RestfulListeners   []listeners.RESTfulListener   `json:"restful" yaml:"restful" toml:"restful"`
}

// LoadConfig loads the configuration from the given path.
func LoadConfig(path string) (*Config, error) {
	ext := filepath.Ext(path)
	switch ext {
	case ".json":
		return loadJSONConfig(path)
	case ".yaml", ".yml":
		return loadYAMLConfig(path)
	case ".toml", ".tml":
		return loadTOMLConfig(path)
	default:
		return nil, fmt.Errorf("unsupported config file format: %s", ext)
	}
}

func loadJSONConfig(path string) (*Config, error) {
	var cfg Config
	content, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	if err := json.Unmarshal(content, &cfg); err != nil {
		return nil, err
	}
	return &cfg, nil
}

func loadYAMLConfig(path string) (*Config, error) {
	var cfg Config
	content, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	if err := yaml.Unmarshal(content, &cfg); err != nil {
		return nil, err
	}
	return &cfg, nil
}

func loadTOMLConfig(path string) (*Config, error) {
	var cfg Config
	content, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	if err := toml.Unmarshal(content, &cfg); err != nil {
		return nil, err
	}
	return &cfg, nil
}
