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

// findMatchingRule is the pure inner implementation with an injected home
// directory. It returns the original stored glob key (not the expanded form)
// so callers can use it directly as a map key for deletion.
//
// Specificity is measured by expanded glob length. This heuristic works well
// in practice but does not account for every structural edge case (e.g. a
// wildcard segment vs. a literal segment of the same depth).
//
// Invalid glob patterns (filepath.ErrBadPattern) are silently skipped; a
// misconfigured rule is ignored rather than blocking all matches.
func findMatchingRule(rules map[string]string, path, home string) (glob, profile string, ok bool) {
	bestGlob := ""
	bestProfile := ""
	bestLen := 0

	for g, p := range rules {
		expanded := expandTilde(g, home)
		matched, err := filepath.Match(expanded, path)
		if err != nil || !matched {
			continue
		}
		// Prefer the most specific (longest) matching glob.
		if len(expanded) > bestLen {
			bestLen = len(expanded)
			bestGlob = g // preserve original stored key
			bestProfile = p
		}
	}

	if bestGlob == "" {
		return "", "", false
	}
	return bestGlob, bestProfile, true
}

// matchRule returns the profile name of the most specific matching rule.
func matchRule(rules map[string]string, path, home string) (string, bool) {
	_, profile, ok := findMatchingRule(rules, path, home)
	return profile, ok
}

// FindMatchingRule returns the original glob key, profile name, and true for
// the most specific rule that matches path. Returns "", "", false if nothing
// matches. Use this over MatchRule when you need the glob key (e.g. to delete
// the rule).
//
// If os.UserHomeDir fails (e.g. no $HOME set), tilde globs are not expanded
// and will silently not match. This is intentional: the fallback is safe and
// only affects unusual environments. Use findMatchingRule directly when you
// need explicit control over the home directory (e.g. in tests).
func FindMatchingRule(rules map[string]string, path string) (glob, profile string, ok bool) {
	home, _ := os.UserHomeDir() // expandTilde handles the empty-home fallback
	return findMatchingRule(rules, path, home)
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
