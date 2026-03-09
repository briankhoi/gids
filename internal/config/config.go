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

// LookupProfile returns the profile with the given name, or nil if not found.
// Use this over FindProfile when the index is not needed.
func (c *AppConfig) LookupProfile(name string) *Profile {
	p, _ := c.FindProfile(name)
	return p
}

// LookupProfileByIdentity returns the profile whose GitName and GitEmail both
// match name and email, or nil if no profile matches. Used by status and
// check commands to identify the active profile from the current git identity.
func (c *AppConfig) LookupProfileByIdentity(name, email string) *Profile {
	for i := range c.Profiles {
		if c.Profiles[i].GitName == name && c.Profiles[i].GitEmail == email {
			p := c.Profiles[i] // copy — avoids returning a pointer into the slice
			return &p
		}
	}
	return nil
}

// DeleteProfile removes the profile with the given name.
// Returns false if not found.
func (c *AppConfig) DeleteProfile(name string) bool {
	_, idx := c.FindProfile(name)
	if idx == -1 {
		return false
	}
	next := make([]Profile, 0, len(c.Profiles)-1)
	next = append(next, c.Profiles[:idx]...)
	next = append(next, c.Profiles[idx+1:]...)
	c.Profiles = next
	return true
}
