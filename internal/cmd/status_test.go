package cmd_test

import (
	"strings"
	"testing"

	"gids/internal/config"
	"gids/internal/git"
	"gids/internal/testutil"
)

// setGitIdentity sets user.name and user.email via git config --local.
func setGitIdentity(t *testing.T, dir, name, email string) {
	t.Helper()
	c := git.New(dir)
	if err := c.ConfigSet("user.name", name); err != nil {
		t.Fatalf("setting user.name: %v", err)
	}
	if err := c.ConfigSet("user.email", email); err != nil {
		t.Fatalf("setting user.email: %v", err)
	}
}

// unsetGitIdentity removes user.name and user.email from git config --local.
func unsetGitIdentity(t *testing.T, dir string) {
	t.Helper()
	c := git.New(dir)
	if err := c.ConfigUnset("user.name"); err != nil {
		t.Fatalf("unsetting user.name: %v", err)
	}
	if err := c.ConfigUnset("user.email"); err != nil {
		t.Fatalf("unsetting user.email: %v", err)
	}
}

// --- gids status ---

func TestStatus_NotInGitRepo(t *testing.T) {
	plainDir := t.TempDir()
	t.Chdir(plainDir)

	cfgDir := t.TempDir()
	cfgPath := writeConfig(t, cfgDir, []config.Profile{workProfile()})

	out, err := execute("status", "--config", cfgPath)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(out, "Not in a git repository") {
		t.Errorf("expected 'Not in a git repository' in output, got: %s", out)
	}
}

func TestStatus_IdentityNotSet(t *testing.T) {
	repoDir := initGitRepo(t)
	// initGitRepo seeds an identity — unset it so the test starts clean.
	unsetGitIdentity(t, repoDir)
	t.Chdir(repoDir)

	cfgDir := t.TempDir()
	cfgPath := writeConfig(t, cfgDir, []config.Profile{workProfile()})

	out, err := execute("status", "--config", cfgPath)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(out, "not set") {
		t.Errorf("expected 'not set' in output, got: %s", out)
	}
	// No rules configured, so source must still be Manual.
	if !strings.Contains(out, "Manual") {
		t.Errorf("expected 'Manual' source in output, got: %s", out)
	}
}

func TestStatus_ManualSource(t *testing.T) {
	repoDir := initGitRepo(t)
	setGitIdentity(t, repoDir, testutil.GitName, testutil.GitEmail)
	t.Chdir(repoDir)

	cfgDir := t.TempDir()
	// No rules configured — source must be "Manual".
	cfgPath := writeConfig(t, cfgDir, []config.Profile{workProfile()})

	out, err := execute("status", "--config", cfgPath)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(out, testutil.GitName) {
		t.Errorf("expected git name %q in output, got: %s", testutil.GitName, out)
	}
	if !strings.Contains(out, testutil.GitEmail) {
		t.Errorf("expected git email %q in output, got: %s", testutil.GitEmail, out)
	}
	if !strings.Contains(out, testutil.ProfileName) {
		t.Errorf("expected profile %q in output, got: %s", testutil.ProfileName, out)
	}
	if !strings.Contains(out, "Manual") {
		t.Errorf("expected 'Manual' source in output, got: %s", out)
	}
}

func TestStatus_RuleSource(t *testing.T) {
	repoDir := initGitRepo(t)
	setGitIdentity(t, repoDir, testutil.GitName, testutil.GitEmail)
	t.Chdir(repoDir)

	cfgDir := t.TempDir()
	cfg := &config.AppConfig{
		Profiles: []config.Profile{workProfile()},
		// Exact-match glob on the repo dir itself.
		Rules: map[string]string{repoDir: testutil.ProfileName},
	}
	cfgPath := writeRuleConfig(t, cfgDir, cfg)

	out, err := execute("status", "--config", cfgPath)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(out, testutil.ProfileName) {
		t.Errorf("expected profile %q in output, got: %s", testutil.ProfileName, out)
	}
	if !strings.Contains(out, "Rule") {
		t.Errorf("expected 'Rule' source in output, got: %s", out)
	}
	if !strings.Contains(out, repoDir) {
		t.Errorf("expected rule glob %q in output, got: %s", repoDir, out)
	}
}

func TestStatus_UnrecognizedIdentity(t *testing.T) {
	repoDir := initGitRepo(t)
	// Set an identity that matches no gids profile.
	setGitIdentity(t, repoDir, "Unknown Person", "unknown@example.com")
	t.Chdir(repoDir)

	cfgDir := t.TempDir()
	cfgPath := writeConfig(t, cfgDir, []config.Profile{workProfile()})

	out, err := execute("status", "--config", cfgPath)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(out, "Unknown Person") {
		t.Errorf("expected git name in output, got: %s", out)
	}
	if !strings.Contains(out, "unrecognized") {
		t.Errorf("expected 'unrecognized' in output, got: %s", out)
	}
}

func TestStatus_OutputFormat(t *testing.T) {
	repoDir := initGitRepo(t)
	setGitIdentity(t, repoDir, testutil.GitName, testutil.GitEmail)
	t.Chdir(repoDir)

	cfgDir := t.TempDir()
	cfgPath := writeConfig(t, cfgDir, []config.Profile{workProfile()})

	out, err := execute("status", "--config", cfgPath)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	for _, want := range []string{"Identity:", "Profile:", "Source:"} {
		if !strings.Contains(out, want) {
			t.Errorf("expected %q label in output, got: %s", want, out)
		}
	}
}
