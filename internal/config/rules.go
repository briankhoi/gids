package config

import (
	"os"
	"path/filepath"
	"strings"
)

// expandTilde replaces a leading "~" in path with homeDir.
// Returns path unchanged if it does not start with "~" or homeDir is empty.
func expandTilde(path, homeDir string) string {
	if homeDir == "" {
		return path
	}
	if path == "~" {
		return homeDir
	}
	if strings.HasPrefix(path, "~/") {
		return homeDir + path[1:]
	}
	return path
}

// matchRule is the pure inner implementation with an injected home directory.
// It iterates all rules and returns the profile name of the most specific
// (longest glob) match, or "", false if nothing matches.
//
// Specificity is measured by expanded glob length. This heuristic works well
// in practice but does not account for every structural edge case (e.g. a
// wildcard segment vs. a literal segment of the same depth). 
//
// Invalid glob patterns (filepath.ErrBadPattern) are silently skipped; a
// misconfigured rule is ignored rather than blocking all matches.
func matchRule(rules map[string]string, path, home string) (string, bool) {
	bestGlob := ""
	bestProfile := ""

	for glob, profile := range rules {
		expanded := expandTilde(glob, home)
		matched, err := filepath.Match(expanded, path)
		if err != nil || !matched {
			continue
		}
		// Prefer the most specific (longest) matching glob.
		if len(expanded) > len(bestGlob) {
			bestGlob = expanded
			bestProfile = profile
		}
	}

	if bestProfile == "" {
		return "", false
	}
	return bestProfile, true
}

// MatchRule finds the most specific matching rule for path.
// Globs stored with a leading "~" are expanded to the current user's home
// directory. Returns the profile name and true if a rule matches, or "",
// false otherwise.
//
// If os.UserHomeDir fails (e.g. no $HOME set), tilde globs are not expanded
// and will silently not match. This is intentional: the fallback is safe and
// only affects unusual environments. Use matchRule directly when you need
// explicit control over the home directory (e.g. in tests).
func MatchRule(rules map[string]string, path string) (string, bool) {
	home, _ := os.UserHomeDir() // expandTilde handles the empty-home fallback
	return matchRule(rules, path, home)
}

// AddRule adds or overwrites the directory-to-profile mapping for glob.
// Initializes the Rules map if it is nil.
func (c *AppConfig) AddRule(glob, profile string) {
	if c.Rules == nil {
		c.Rules = make(map[string]string)
	}
	c.Rules[glob] = profile
}

// RemoveRule deletes the mapping for glob.
// Returns true if the rule existed, false otherwise.
func (c *AppConfig) RemoveRule(glob string) bool {
	if c.Rules == nil {
		return false
	}
	_, existed := c.Rules[glob]
	delete(c.Rules, glob) // delete of missing key is a no-op
	return existed
}
