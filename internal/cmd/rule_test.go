package cmd_test

import (
	"path/filepath"
	"strings"
	"testing"

	"gids/internal/config"
	"gids/internal/testutil"
)

// writeRuleConfig saves a full AppConfig (profiles + rules) to a temp file and
// returns the path. Use this instead of writeConfig when rules must be pre-set.
func writeRuleConfig(t *testing.T, dir string, cfg *config.AppConfig) string {
	t.Helper()
	path := filepath.Join(dir, "config.yaml")
	if err := config.Save(cfg, path); err != nil {
		t.Fatalf("writeRuleConfig: %v", err)
	}
	return path
}

// personalProfile returns a minimal Personal profile for tests.
func personalProfile() config.Profile {
	return config.Profile{
		Name:     testutil.ProfileName2,
		GitName:  testutil.GitName,
		GitEmail: testutil.GitEmail2,
	}
}

// --- rule list ---

func TestRuleList_NoRules(t *testing.T) {
	dir := t.TempDir()
	cfgPath := writeConfig(t, dir, []config.Profile{})

	out, err := execute("rule", "list", "--config", cfgPath)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(out, "No rules configured") {
		t.Errorf("expected 'No rules configured', got: %s", out)
	}
}

func TestRuleList_WithRules(t *testing.T) {
	dir := t.TempDir()
	cfg := &config.AppConfig{
		Profiles: []config.Profile{workProfile()},
		Rules:    map[string]string{testutil.RuleGlobWorkTilde: testutil.ProfileName},
	}
	cfgPath := writeRuleConfig(t, dir, cfg)

	out, err := execute("rule", "list", "--config", cfgPath)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(out, testutil.RuleGlobWorkTilde) {
		t.Errorf("expected glob %q in output, got: %s", testutil.RuleGlobWorkTilde, out)
	}
	if !strings.Contains(out, testutil.ProfileName) {
		t.Errorf("expected profile %q in output, got: %s", testutil.ProfileName, out)
	}
}

func TestRuleList_MultipleRules(t *testing.T) {
	dir := t.TempDir()
	cfg := &config.AppConfig{
		Profiles: []config.Profile{workProfile(), personalProfile()},
		Rules: map[string]string{
			testutil.RuleGlobWorkTilde:     testutil.ProfileName,
			testutil.RuleGlobPersonalTilde: testutil.ProfileName2,
		},
	}
	cfgPath := writeRuleConfig(t, dir, cfg)

	out, err := execute("rule", "list", "--config", cfgPath)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(out, testutil.ProfileName) {
		t.Errorf("expected %q in output, got: %s", testutil.ProfileName, out)
	}
	if !strings.Contains(out, testutil.ProfileName2) {
		t.Errorf("expected %q in output, got: %s", testutil.ProfileName2, out)
	}
}

// --- rule add ---

func TestRuleAdd_HappyPath(t *testing.T) {
	dir := t.TempDir()
	cfgPath := writeConfig(t, dir, []config.Profile{workProfile()})

	out, err := execute("rule", "add", testutil.RuleGlobWorkTilde, testutil.ProfileName, "--config", cfgPath)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(out, "Added rule:") {
		t.Errorf("expected 'Added rule:' in output, got: %s", out)
	}

	// Verify persisted.
	cfg, err := config.Load(cfgPath)
	if err != nil {
		t.Fatalf("loading config: %v", err)
	}
	if cfg.Rules[testutil.RuleGlobWorkTilde] != testutil.ProfileName {
		t.Errorf("rule not persisted: Rules[%q] = %q", testutil.RuleGlobWorkTilde, cfg.Rules[testutil.RuleGlobWorkTilde])
	}
}

func TestRuleAdd_OutputIncludesGlobAndProfile(t *testing.T) {
	dir := t.TempDir()
	cfgPath := writeConfig(t, dir, []config.Profile{workProfile()})

	out, err := execute("rule", "add", testutil.RuleGlobWorkTilde, testutil.ProfileName, "--config", cfgPath)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(out, testutil.RuleGlobWorkTilde) {
		t.Errorf("expected glob %q in output, got: %s", testutil.RuleGlobWorkTilde, out)
	}
	if !strings.Contains(out, testutil.ProfileName) {
		t.Errorf("expected profile %q in output, got: %s", testutil.ProfileName, out)
	}
}

func TestRuleAdd_InvalidGlobRejected(t *testing.T) {
	dir := t.TempDir()
	cfgPath := writeConfig(t, dir, []config.Profile{workProfile()})

	_, err := execute("rule", "add", "[unclosed", testutil.ProfileName, "--config", cfgPath)
	if err == nil {
		t.Fatal("expected error for invalid glob pattern")
	}
	if !strings.Contains(err.Error(), "invalid glob") {
		t.Errorf("expected 'invalid glob' in error, got: %v", err)
	}
}

func TestRuleAdd_ProfileNotFound(t *testing.T) {
	dir := t.TempDir()
	cfgPath := writeConfig(t, dir, []config.Profile{})

	_, err := execute("rule", "add", testutil.RuleGlobWorkTilde, "NonExistent", "--config", cfgPath)
	if err == nil {
		t.Fatal("expected error for missing profile")
	}
	if !strings.Contains(err.Error(), "not found") {
		t.Errorf("expected 'not found' in error, got: %v", err)
	}
}

func TestRuleAdd_OverwritesExistingRule(t *testing.T) {
	dir := t.TempDir()
	cfg := &config.AppConfig{
		Profiles: []config.Profile{workProfile(), personalProfile()},
		Rules:    map[string]string{testutil.RuleGlobWorkTilde: testutil.ProfileName},
	}
	cfgPath := writeRuleConfig(t, dir, cfg)

	_, err := execute("rule", "add", testutil.RuleGlobWorkTilde, testutil.ProfileName2, "--config", cfgPath)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	loaded, err := config.Load(cfgPath)
	if err != nil {
		t.Fatalf("loading config: %v", err)
	}
	if loaded.Rules[testutil.RuleGlobWorkTilde] != testutil.ProfileName2 {
		t.Errorf("expected overwrite to %q, got %q", testutil.ProfileName2, loaded.Rules[testutil.RuleGlobWorkTilde])
	}
}

// --- rule remove ---

func TestRuleRemove_ExplicitGlob(t *testing.T) {
	dir := t.TempDir()
	cfg := &config.AppConfig{
		Profiles: []config.Profile{workProfile()},
		Rules:    map[string]string{testutil.RuleGlobWorkTilde: testutil.ProfileName},
	}
	cfgPath := writeRuleConfig(t, dir, cfg)

	out, err := execute("rule", "remove", testutil.RuleGlobWorkTilde, "--config", cfgPath)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(out, "Removed rule:") {
		t.Errorf("expected 'Removed rule:' in output, got: %s", out)
	}

	loaded, err := config.Load(cfgPath)
	if err != nil {
		t.Fatalf("loading config: %v", err)
	}
	if _, exists := loaded.Rules[testutil.RuleGlobWorkTilde]; exists {
		t.Error("rule still present after removal")
	}
}

func TestRuleRemove_OutputIncludesGlobAndProfile(t *testing.T) {
	dir := t.TempDir()
	cfg := &config.AppConfig{
		Profiles: []config.Profile{workProfile()},
		Rules:    map[string]string{testutil.RuleGlobWorkTilde: testutil.ProfileName},
	}
	cfgPath := writeRuleConfig(t, dir, cfg)

	out, err := execute("rule", "remove", testutil.RuleGlobWorkTilde, "--config", cfgPath)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(out, testutil.RuleGlobWorkTilde) {
		t.Errorf("expected glob %q in output, got: %s", testutil.RuleGlobWorkTilde, out)
	}
	if !strings.Contains(out, testutil.ProfileName) {
		t.Errorf("expected profile %q in output, got: %s", testutil.ProfileName, out)
	}
}

func TestRuleRemove_RuleNotFound(t *testing.T) {
	dir := t.TempDir()
	cfgPath := writeConfig(t, dir, []config.Profile{workProfile()})

	_, err := execute("rule", "remove", testutil.RuleGlobWorkTilde, "--config", cfgPath)
	if err == nil {
		t.Fatal("expected error for missing rule")
	}
	if !strings.Contains(err.Error(), "no rule") {
		t.Errorf("expected 'no rule' in error, got: %v", err)
	}
}

func TestRuleRemove_NoCwdMatch(t *testing.T) {
	// Chdir to a temp dir that has no matching rule.
	plainDir := t.TempDir()
	t.Chdir(plainDir)

	dir := t.TempDir()
	cfgPath := writeConfig(t, dir, []config.Profile{workProfile()})

	_, err := execute("rule", "remove", "--config", cfgPath)
	if err == nil {
		t.Fatal("expected error when no rule matches cwd")
	}
	if !strings.Contains(err.Error(), "no rule") {
		t.Errorf("expected 'no rule' in error, got: %v", err)
	}
}
