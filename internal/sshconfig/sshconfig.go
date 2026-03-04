// Package sshconfig parses SSH config files into a simple Host slice.
package sshconfig

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	gossh "github.com/kevinburke/ssh_config"
)

// Host holds the fields from a parsed SSH config Host block that are
// relevant for creating a gids profile.
type Host struct {
	Pattern      string // Host line value (e.g. "github.com-work")
	HostName     string // HostName directive
	User         string // User directive
	IdentityFile string // First IdentityFile directive
}

// DefaultConfigPaths returns candidate SSH config file paths for the current
// platform in priority order. Paths are not guaranteed to exist.
func DefaultConfigPaths() []string {
	if runtime.GOOS == "windows" {
		userProfile := os.Getenv("USERPROFILE")
		programData := os.Getenv("ProgramData")
		return []string{
			filepath.Join(userProfile, ".ssh", "config"),
			filepath.Join(programData, "ssh", "ssh_config"),
		}
	}
	home, _ := os.UserHomeDir()
	return []string{
		filepath.Join(home, ".ssh", "config"),
		"/etc/ssh/ssh_config",
	}
}

// ParseFile reads the SSH config at path and returns one Host per non-wildcard
// Host block. Include directives are followed automatically by the underlying
// library. The first IdentityFile directive per block wins.
func ParseFile(path string) ([]Host, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("cannot open SSH config: %s: %w", path, err)
	}
	defer f.Close()

	cfg, err := gossh.Decode(f)
	if err != nil {
		return nil, fmt.Errorf("cannot parse SSH config: %s: %w", path, err)
	}

	var hosts []Host
	for _, h := range cfg.Hosts {
		// Collect all pattern strings for this block.
		var patterns []string
		for _, p := range h.Patterns {
			patterns = append(patterns, p.String())
		}

		// Skip blocks where any pattern contains a wildcard — these are
		// global defaults or multi-host matchers, not individual identities.
		isWildcard := false
		for _, pat := range patterns {
			if strings.ContainsAny(pat, "*?") {
				isWildcard = true
				break
			}
		}
		if isWildcard {
			continue
		}

		// Use the first pattern as the profile name.
		if len(patterns) == 0 {
			continue
		}
		pattern := patterns[0]

		var hostName, user, identityFile string
		for _, node := range h.Nodes {
			kv, ok := node.(*gossh.KV)
			if !ok {
				continue
			}
			switch strings.ToLower(kv.Key) {
			case "hostname":
				if hostName == "" {
					hostName = kv.Value
				}
			case "user":
				if user == "" {
					user = kv.Value
				}
			case "identityfile":
				if identityFile == "" {
					identityFile = kv.Value
				}
			}
		}

		hosts = append(hosts, Host{
			Pattern:      pattern,
			HostName:     hostName,
			User:         user,
			IdentityFile: identityFile,
		})
	}
	return hosts, nil
}
