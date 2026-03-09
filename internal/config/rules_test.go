package config

import (
	"os"
	"path/filepath"
	"testing"

	"gids/internal/testutil"
)

// --- expandTilde ---

func TestExpandTilde(t *testing.T) {
	tests := []struct {
		name string
		path string
		home string
		want string
	}{
		{"tilde slash prefix", "~/work/foo", testutil.RuleHome, testutil.RuleHome + "/work/foo"},
		{"exact tilde", "~", testutil.RuleHome, testutil.RuleHome},
		{"absolute path unchanged", "/etc/ssh", testutil.RuleHome, "/etc/ssh"},
		{"empty home returns path unchanged", "~/work", "", "~/work"},
		{"empty path unchanged", "", testutil.RuleHome, ""},
		// ~username is a shell convention; Go stdlib does not expand it and
		// neither do we — document this intentional non-support.
		{"tilde with username not expanded", "~otheruser/work", testutil.RuleHome, "~otheruser/work"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := expandTilde(tc.path, tc.home)
			if got != tc.want {
				t.Errorf("expandTilde(%q, %q) = %q, want %q", tc.path, tc.home, got, tc.want)
			}
		})
	}
}

// --- matchRule ---

func TestMatchRule_ExactPath(t *testing.T) {
	rules := map[string]string{
		testutil.RulePathExact: testutil.ProfileName,
	}
	got, ok := matchRule(rules, testutil.RulePathExact, testutil.RuleHome)
	if !ok || got != testutil.ProfileName {
		t.Errorf("matchRule = (%q, %v), want (%q, true)", got, ok, testutil.ProfileName)
	}
}

func TestMatchRule_WildcardGlob(t *testing.T) {
	rules := map[string]string{
		testutil.RuleGlobWork: testutil.ProfileName,
	}
	got, ok := matchRule(rules, testutil.RulePathWork, testutil.RuleHome)
	if !ok || got != testutil.ProfileName {
		t.Errorf("matchRule = (%q, %v), want (%q, true)", got, ok, testutil.ProfileName)
	}
}

func TestMatchRule_TildeGlobExpanded(t *testing.T) {
	rules := map[string]string{
		testutil.RuleGlobWorkTilde: testutil.ProfileName,
	}
	got, ok := matchRule(rules, testutil.RulePathWork, testutil.RuleHome)
	if !ok || got != testutil.ProfileName {
		t.Errorf("matchRule = (%q, %v), want (%q, true)", got, ok, testutil.ProfileName)
	}
}

func TestMatchRule_NoMatch(t *testing.T) {
	rules := map[string]string{
		testutil.RuleGlobWork: testutil.ProfileName,
	}
	_, ok := matchRule(rules, testutil.RulePathPersonal, testutil.RuleHome)
	if ok {
		t.Error("expected no match for unrelated path")
	}
}

func TestMatchRule_EmptyRules(t *testing.T) {
	_, ok := matchRule(nil, "/any/path", testutil.RuleHome)
	if ok {
		t.Error("expected no match for empty rules")
	}
}

func TestMatchRule_MostSpecificWins(t *testing.T) {
	rules := map[string]string{
		testutil.RuleGlobWork: testutil.ProfileName,
		testutil.RuleGlobOSS:  testutil.ProfileName3,
	}
	got, ok := matchRule(rules, testutil.RulePathOSS, testutil.RuleHome)
	if !ok || got != testutil.ProfileName3 {
		t.Errorf("matchRule = (%q, %v), want (%q, true)", got, ok, testutil.ProfileName3)
	}
}

func TestMatchRule_GlobBasePathDoesNotMatch(t *testing.T) {
	// ~/work/* should NOT match ~/work itself — the wildcard requires a child segment.
	rules := map[string]string{
		testutil.RuleGlobWork: testutil.ProfileName,
	}
	_, ok := matchRule(rules, testutil.RulePathExact, testutil.RuleHome)
	if ok {
		t.Error("expected no match: path is the glob base, not a child")
	}
}

func TestMatchRule_InvalidGlob_Skipped(t *testing.T) {
	// A syntactically invalid glob must not panic — it is silently skipped.
	rules := map[string]string{
		"[invalid": testutil.ProfileName,
	}
	_, ok := matchRule(rules, "[invalid", testutil.RuleHome)
	if ok {
		t.Error("expected no match for invalid glob pattern")
	}
}

// --- MatchRule (public, uses real home dir) ---

func TestMatchRule_Public_TildeExpansion(t *testing.T) {
	home, err := os.UserHomeDir()
	if err != nil {
		t.Skip("cannot determine home directory")
	}
	rules := map[string]string{
		testutil.RuleGlobWorkTilde: testutil.ProfileName,
	}
	path := filepath.Join(home, "work", "myproject")
	got, ok := MatchRule(rules, path)
	if !ok || got != testutil.ProfileName {
		t.Errorf("MatchRule = (%q, %v), want (%q, true)", got, ok, testutil.ProfileName)
	}
}

// --- AppConfig.AddRule / RemoveRule ---

func TestAddRule_NewRule(t *testing.T) {
	cfg := &AppConfig{}
	cfg.AddRule(testutil.RuleGlobWorkTilde, testutil.ProfileName)
	if cfg.Rules[testutil.RuleGlobWorkTilde] != testutil.ProfileName {
		t.Errorf("Rules[%q] = %q, want %q", testutil.RuleGlobWorkTilde, cfg.Rules[testutil.RuleGlobWorkTilde], testutil.ProfileName)
	}
}

func TestAddRule_OverwritesExisting(t *testing.T) {
	cfg := &AppConfig{Rules: map[string]string{testutil.RuleGlobWorkTilde: "OldProfile"}}
	cfg.AddRule(testutil.RuleGlobWorkTilde, testutil.ProfileName2)
	if cfg.Rules[testutil.RuleGlobWorkTilde] != testutil.ProfileName2 {
		t.Errorf("Rules[%q] = %q, want %q", testutil.RuleGlobWorkTilde, cfg.Rules[testutil.RuleGlobWorkTilde], testutil.ProfileName2)
	}
}

func TestAddRule_InitializesNilMap(t *testing.T) {
	cfg := &AppConfig{Rules: nil}
	cfg.AddRule(testutil.RuleGlobWorkTilde, testutil.ProfileName)
	if cfg.Rules == nil {
		t.Fatal("expected Rules map to be initialized")
	}
	if cfg.Rules[testutil.RuleGlobWorkTilde] != testutil.ProfileName {
		t.Errorf("Rules[%q] = %q, want %q", testutil.RuleGlobWorkTilde, cfg.Rules[testutil.RuleGlobWorkTilde], testutil.ProfileName)
	}
}

func TestRemoveRule_ExistingRule(t *testing.T) {
	cfg := &AppConfig{Rules: map[string]string{testutil.RuleGlobWorkTilde: testutil.ProfileName}}
	ok := cfg.RemoveRule(testutil.RuleGlobWorkTilde)
	if !ok {
		t.Fatal("expected true when removing existing rule")
	}
	if _, exists := cfg.Rules[testutil.RuleGlobWorkTilde]; exists {
		t.Error("rule still present after removal")
	}
}

func TestRemoveRule_NonExistentRule(t *testing.T) {
	cfg := &AppConfig{Rules: map[string]string{testutil.RuleGlobWorkTilde: testutil.ProfileName}}
	ok := cfg.RemoveRule("~/personal/*")
	if ok {
		t.Error("expected false when removing non-existent rule")
	}
}

func TestRemoveRule_NilRules(t *testing.T) {
	cfg := &AppConfig{Rules: nil}
	ok := cfg.RemoveRule(testutil.RuleGlobWorkTilde)
	if ok {
		t.Error("expected false when rules map is nil")
	}
}
