package config

import (
	"errors"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

// DefaultConfigPath returns $UserConfigDir/gids/config.yaml.
func DefaultConfigPath() (string, error) {
	base, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(base, "gids", "config.yaml"), nil
}

func resolveConfigPath(configPath string) (string, error) {
	if configPath != "" {
		return configPath, nil
	}
	return DefaultConfigPath()
}

// Load reads AppConfig from configPath.
// Pass "" to use DefaultConfigPath.
// Returns an empty AppConfig (not an error) if the file does not exist.
func Load(configPath string) (*AppConfig, error) {
	path, err := resolveConfigPath(configPath)
	if err != nil {
		return nil, err
	}

	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return &AppConfig{}, nil
		}
		return nil, err
	}

	var cfg AppConfig
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, err
	}
	return &cfg, nil
}

// Save writes cfg to configPath, creating parent directories as needed.
// Pass "" to use DefaultConfigPath.
func Save(cfg *AppConfig, configPath string) error {
	path, err := resolveConfigPath(configPath)
	if err != nil {
		return err
	}

	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}

	data, err := yaml.Marshal(cfg)
	if err != nil {
		return err
	}

	return os.WriteFile(path, data, 0o600)
}

// FindProfile returns the profile with the given name and its index.
// Returns nil, -1 if not found.
func (c *AppConfig) FindProfile(name string) (*Profile, int) {
	for i := range c.Profiles {
		if c.Profiles[i].Name == name {
			return &c.Profiles[i], i
		}
	}
	return nil, -1
}

// DeleteProfile removes the profile with the given name.
// Returns false if not found.
func (c *AppConfig) DeleteProfile(name string) bool {
	_, idx := c.FindProfile(name)
	if idx == -1 {
		return false
	}
	c.Profiles = append(c.Profiles[:idx], c.Profiles[idx+1:]...)
	return true
}
