package config

import (
	"errors"
	"fmt"
	"path"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strings"
	"unicode"
)

var profileNameRE = regexp.MustCompile(`^[A-Za-z0-9](?:[A-Za-z0-9._-]{0,62}[A-Za-z0-9_-])?$`)

// windowsReserved can't be file names on Windows; profiles become file names.
var windowsReserved = map[string]bool{"CON": true, "PRN": true, "AUX": true, "NUL": true}

func init() {
	for i := 1; i <= 9; i++ {
		windowsReserved[fmt.Sprintf("COM%d", i)] = true
		windowsReserved[fmt.Sprintf("LPT%d", i)] = true
	}
}

// ValidateProfileName: 1-64 chars of letters, digits, '.', '_', '-', starting
// with a letter or digit and not ending with '.'; not a Windows device name.
func ValidateProfileName(s string) error {
	if !profileNameRE.MatchString(s) {
		return errors.New("profile names use letters, digits, '.', '_' or '-' (max 64), start with a letter or digit, and don't end with '.'")
	}
	if windowsReserved[strings.ToUpper(s)] {
		return fmt.Errorf("%q is a reserved device name on Windows", s)
	}
	return nil
}

// ValidateGitName: non-empty, no control characters, no '<' or '>' (they'd
// break the "Name <email>" author format).
func ValidateGitName(s string) error {
	if strings.TrimSpace(s) == "" {
		return errors.New("git name is required")
	}
	if s != strings.TrimSpace(s) {
		return errors.New("git name has leading or trailing spaces")
	}
	if err := noControl(s); err != nil {
		return err
	}
	if strings.ContainsAny(s, "<>") {
		return errors.New("git name can't contain '<' or '>'")
	}
	return nil
}

// ValidateEmail: exactly one '@' with something on both sides, no spaces,
// no '<' or '>', no control characters. Deliberately not full RFC 5322.
func ValidateEmail(s string) error {
	if s == "" {
		return errors.New("email is required")
	}
	if err := noControl(s); err != nil {
		return err
	}
	if strings.ContainsAny(s, " \t<>") {
		return errors.New("email can't contain spaces, '<' or '>'")
	}
	at := strings.Count(s, "@")
	i := strings.IndexByte(s, '@')
	if at != 1 || i == 0 || i == len(s)-1 {
		return errors.New("email must look like name@host")
	}
	return nil
}

// ValidateUsername: optional; no whitespace or control characters.
func ValidateUsername(s string) error {
	if s == "" {
		return nil
	}
	if err := noControl(s); err != nil {
		return err
	}
	if strings.IndexFunc(s, unicode.IsSpace) >= 0 {
		return errors.New("username can't contain spaces")
	}
	return nil
}

// ValidateKeyPath: optional SSH key path. Must be absolute or start with "~/"
// (a relative path would resolve against whatever directory git runs in).
func ValidateKeyPath(s string) error {
	if s == "" {
		return nil
	}
	if err := noControl(s); err != nil {
		return err
	}
	if !strings.HasPrefix(s, "~/") && !filepath.IsAbs(s) {
		return errors.New("SSH key path must be absolute or start with ~/")
	}
	return nil
}

// ValidateSigningKey: optional; a GPG key id, an SSH key path, or "key::…".
func ValidateSigningKey(s string) error {
	if s == "" {
		return nil
	}
	if err := noControl(s); err != nil {
		return err
	}
	if s != strings.TrimSpace(s) {
		return errors.New("signing key has leading or trailing spaces")
	}
	return nil
}

// Validate checks all fields of p.
func (p Profile) Validate() error {
	return errors.Join(
		ValidateProfileName(p.Name),
		ValidateGitName(p.GitName),
		ValidateEmail(p.GitEmail),
		ValidateUsername(p.Username),
		ValidateKeyPath(p.SSHKey),
		ValidateSigningKey(p.SigningKey),
	)
}

func noControl(s string) error {
	for _, r := range s {
		if unicode.IsControl(r) {
			return errors.New("contains a control character (newline, tab, escape…)")
		}
	}
	return nil
}

// ---- rules ----

// NormalizeRule turns user input naming a directory into the stored rule
// key: absolute, cleaned, '/'-separated, shown as "~/…" when inside home, and
// ending in "/" so git's gitdir matching covers everything beneath it.
// Relative input resolves against cwd. Git glob characters are kept.
func NormalizeRule(input, cwd, home string) (string, error) {
	s := strings.TrimSpace(input)
	if s == "" {
		return "", errors.New("directory is required")
	}
	if err := noControl(s); err != nil {
		return "", err
	}
	if runtime.GOOS == "windows" {
		s = filepath.ToSlash(s)
	}
	if strings.ContainsAny(s, `"\`) {
		return "", errors.New(`directory can't contain '"' or '\'`)
	}
	switch {
	case s == "~":
		s = home
	case strings.HasPrefix(s, "~/"):
		s = filepath.Join(home, s[2:])
	case strings.HasPrefix(s, "~"):
		return "", errors.New("only ~/ (your own home) is supported, not ~user")
	case !filepath.IsAbs(s):
		s = filepath.Join(cwd, s)
	}
	s = filepath.ToSlash(filepath.Clean(s))
	for _, h := range HomeForms(home) {
		if s == h {
			s = "~"
			break
		}
		if strings.HasPrefix(s, h+"/") {
			s = "~" + s[len(h):]
			break
		}
	}
	if !strings.HasSuffix(s, "/") {
		s += "/"
	}
	return s, nil
}

// HomeForms is home as given and, if different, with symlinks resolved
// ('/'-separated). Git reports repository paths with symlinks resolved (e.g.
// macOS /var → /private/var), so both forms must count as "inside home".
func HomeForms(home string) []string {
	h := filepath.ToSlash(filepath.Clean(home))
	forms := []string{h}
	if r, err := filepath.EvalSymlinks(home); err == nil {
		if r = filepath.ToSlash(filepath.Clean(r)); r != h {
			forms = append(forms, r)
		}
	}
	return forms
}

// ValidateRuleKey checks that a stored rule key is in normalized form, without
// depending on the current home directory.
func ValidateRuleKey(k string) error {
	if err := noControl(k); err != nil {
		return err
	}
	if strings.ContainsAny(k, `"\`) {
		return errors.New(`can't contain '"' or '\'`)
	}
	if !strings.HasSuffix(k, "/") {
		return errors.New("must end with '/'")
	}
	body := strings.TrimSuffix(k, "/")
	switch {
	case k == "~/" || k == "/":
		return nil
	case strings.HasPrefix(k, "~/"), strings.HasPrefix(k, "/"), isDrivePath(k):
	default:
		return errors.New("must be absolute or start with ~/")
	}
	if path.Clean(body) != body {
		return errors.New("must be a clean path (no '.', '..' or '//')")
	}
	return nil
}

func isDrivePath(s string) bool {
	return len(s) >= 3 && s[1] == ':' && s[2] == '/' && unicode.IsLetter(rune(s[0]))
}

// ExpandRule replaces a leading "~" in a rule key with home ('/'-separated).
func ExpandRule(k, home string) string {
	if strings.HasPrefix(k, "~/") {
		return filepath.ToSlash(filepath.Clean(home)) + k[1:]
	}
	return k
}

// SortedRules returns rule keys least- to most-specific. Two rules can both
// match a repo only if one directory contains the other, and the contained
// one has more path segments; so sorting by segment count (with ~ expanded)
// and rendering in that order makes the most specific rule win, because git
// applies includes in order and the last match wins. Ties sort lexically.
func SortedRules(rules map[string]string, home string) []string {
	keys := make([]string, 0, len(rules))
	for k := range rules {
		keys = append(keys, k)
	}
	depth := func(k string) int {
		return strings.Count(strings.TrimSuffix(ExpandRule(k, home), "/"), "/")
	}
	sort.Slice(keys, func(i, j int) bool {
		if di, dj := depth(keys[i]), depth(keys[j]); di != dj {
			return di < dj
		}
		return keys[i] < keys[j]
	})
	return keys
}
