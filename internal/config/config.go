// Package config holds gids' source of truth: profiles and directory rules,
// persisted as config.yaml, plus the filesystem locations gids works with.
package config

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"

	"gids/internal/fsx"
)

// Profile is one Git identity.
type Profile struct {
	Name       string `yaml:"name"`
	GitName    string `yaml:"git_name"`
	GitEmail   string `yaml:"git_email"`
	Username   string `yaml:"username,omitempty"`    // credential.username
	SSHKey     string `yaml:"ssh_key,omitempty"`     // core.sshCommand
	SigningKey string `yaml:"signing_key,omitempty"` // user.signingKey
}

// Ident is the "Name <email>" form git uses for authors.
func (p Profile) Ident() string { return p.GitName + " <" + p.GitEmail + ">" }

// Config is the contents of config.yaml.
type Config struct {
	Profiles []Profile         `yaml:"profiles"`
	Rules    map[string]string `yaml:"rules,omitempty"` // normalized dir pattern -> profile name
}

// Paths are the files gids reads and writes.
type Paths struct {
	Home            string // user's home directory
	Dir             string // gids directory
	Config          string // Dir/config.yaml
	Gitconfig       string // Dir/gitconfig (generated includeIf list)
	ProfilesDir     string // Dir/profiles (generated)
	HooksDir        string // Dir/hooks (generated)
	GlobalGitconfig string // the file `git config --global` uses
}

// ResolvePaths computes Paths from the environment:
// $XDG_CONFIG_HOME/gids, else ~/.config/gids (%APPDATA%\gids on Windows);
// the global git config is $GIT_CONFIG_GLOBAL if set, else ~/.gitconfig.
// Every path must be absolute: a relative $HOME or $GIT_CONFIG_GLOBAL would
// make gids write into whatever directory it runs in (e.g. a repo, from a
// hook), so those are errors and a relative $XDG_CONFIG_HOME/$APPDATA is
// ignored (as the XDG spec requires).
func ResolvePaths() (Paths, error) {
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return Paths{}, errors.New("can't determine your home directory (is $HOME set?)")
	}
	if !filepath.IsAbs(home) {
		return Paths{}, fmt.Errorf("your home directory %q isn't an absolute path (check $HOME)", home)
	}
	var dir string
	if x := os.Getenv("XDG_CONFIG_HOME"); x != "" && filepath.IsAbs(x) {
		dir = filepath.Join(x, "gids")
	} else if a := os.Getenv("APPDATA"); runtime.GOOS == "windows" && a != "" && filepath.IsAbs(a) {
		dir = filepath.Join(a, "gids")
	} else {
		dir = filepath.Join(home, ".config", "gids")
	}
	global := os.Getenv("GIT_CONFIG_GLOBAL")
	if global == "" {
		global = filepath.Join(home, ".gitconfig")
	} else if !filepath.IsAbs(global) {
		return Paths{}, fmt.Errorf("$GIT_CONFIG_GLOBAL %q isn't an absolute path; gids won't guess which file you mean", global)
	}
	return Paths{
		Home:            home,
		Dir:             dir,
		Config:          filepath.Join(dir, "config.yaml"),
		Gitconfig:       filepath.Join(dir, "gitconfig"),
		ProfilesDir:     filepath.Join(dir, "profiles"),
		HooksDir:        filepath.Join(dir, "hooks"),
		GlobalGitconfig: global,
	}, nil
}

// Load reads and validates config.yaml. A missing or empty file is an empty
// config. Unknown keys and invalid contents are errors that name the file;
// gids never rewrites a config it couldn't read.
func Load(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return &Config{}, nil
	}
	if err != nil {
		return nil, err
	}
	cfg := &Config{}
	dec := yaml.NewDecoder(bytes.NewReader(data))
	dec.KnownFields(true)
	if err := dec.Decode(cfg); err != nil && !errors.Is(err, io.EOF) {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	if err := cfg.Validate(); err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	return cfg, nil
}

// Save validates cfg and atomically writes it to path (mode 0600).
func Save(path string, cfg *Config) error {
	if err := cfg.Validate(); err != nil {
		return err
	}
	data, err := yaml.Marshal(cfg)
	if err != nil {
		return err
	}
	return fsx.WriteAtomic(path, data, 0o600)
}

// Validate checks every invariant config.yaml must satisfy (SPEC §11 I1).
func (c *Config) Validate() error {
	for i, p := range c.Profiles {
		if err := p.Validate(); err != nil {
			return fmt.Errorf("profile %q: %w", p.Name, err)
		}
		for _, q := range c.Profiles[:i] {
			if strings.EqualFold(p.Name, q.Name) {
				return fmt.Errorf("duplicate profile name %q (names are case-insensitive)", p.Name)
			}
		}
	}
	for dir, name := range c.Rules {
		if err := ValidateRuleKey(dir); err != nil {
			return fmt.Errorf("rule %q: %w", dir, err)
		}
		if c.Profile(name) == nil {
			return fmt.Errorf("rule %q points to unknown profile %q", dir, name)
		}
	}
	return nil
}

// Profile returns the profile named name (exact match), or nil.
func (c *Config) Profile(name string) *Profile {
	for i := range c.Profiles {
		if c.Profiles[i].Name == name {
			return &c.Profiles[i]
		}
	}
	return nil
}

// ProfileFold returns the profile whose name matches name case-insensitively.
func (c *Config) ProfileFold(name string) *Profile {
	for i := range c.Profiles {
		if strings.EqualFold(c.Profiles[i].Name, name) {
			return &c.Profiles[i]
		}
	}
	return nil
}

// ProfileByIdentity returns the first profile with exactly this name and email.
func (c *Config) ProfileByIdentity(name, email string) *Profile {
	for i := range c.Profiles {
		if c.Profiles[i].GitName == name && strings.EqualFold(c.Profiles[i].GitEmail, email) {
			return &c.Profiles[i]
		}
	}
	return nil
}

// AddProfile appends p after checking it and name uniqueness.
func (c *Config) AddProfile(p Profile) error {
	if err := p.Validate(); err != nil {
		return err
	}
	if c.ProfileFold(p.Name) != nil {
		return fmt.Errorf("a profile named %q already exists", p.Name)
	}
	c.Profiles = append(c.Profiles, p)
	return nil
}

// ReplaceProfile swaps the profile named old for p, renaming rules if the
// name changed.
func (c *Config) ReplaceProfile(old string, p Profile) error {
	if err := p.Validate(); err != nil {
		return err
	}
	idx := -1
	for i := range c.Profiles {
		if c.Profiles[i].Name == old {
			idx = i
		} else if strings.EqualFold(c.Profiles[i].Name, p.Name) {
			return fmt.Errorf("a profile named %q already exists", p.Name)
		}
	}
	if idx < 0 {
		return fmt.Errorf("profile %q not found", old)
	}
	c.Profiles[idx] = p
	for dir, name := range c.Rules {
		if name == old {
			c.Rules[dir] = p.Name
		}
	}
	return nil
}

// RulesFor lists the rule dirs that point to profile name, sorted lexically.
func (c *Config) RulesFor(name string) []string {
	var out []string
	for dir, n := range c.Rules {
		if n == name {
			out = append(out, dir)
		}
	}
	sort.Strings(out)
	return out
}

// DeleteProfile removes the profile and every rule pointing to it.
func (c *Config) DeleteProfile(name string) {
	next := c.Profiles[:0:0]
	for _, p := range c.Profiles {
		if p.Name != name {
			next = append(next, p)
		}
	}
	c.Profiles = next
	for dir, n := range c.Rules {
		if n == name {
			delete(c.Rules, dir)
		}
	}
}

// SetRule maps dir (already normalized) to profile.
func (c *Config) SetRule(dir, profile string) {
	if c.Rules == nil {
		c.Rules = map[string]string{}
	}
	c.Rules[dir] = profile
}
