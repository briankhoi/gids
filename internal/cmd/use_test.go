package cmd_test

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"gids/internal/config"
	"gids/internal/git"
	"gids/internal/testutil"
)

// initGitRepo creates a temp dir, runs git init, and returns the path.
func initGitRepo(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	cmd := exec.Command("git", "init", dir)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git init: %v\n%s", err, out)
	}
	// Seed a user identity so git doesn't complain later.
	for _, kv := range [][2]string{
		{"user.email", "seed@test.com"},
		{"user.name", "Seed"},
	} {
		c := exec.Command("git", "-C", dir, "config", "--local", kv[0], kv[1])
		if out, err := c.CombinedOutput(); err != nil {
			t.Fatalf("git config %s: %v\n%s", kv[0], err, out)
		}
	}
	return dir
}

// --- gids use ---

func TestUse_AppliesProfile(t *testing.T) {
	repoDir := initGitRepo(t)
	t.Chdir(repoDir)

	cfgDir := t.TempDir()
	cfgPath := writeConfig(t, cfgDir, []config.Profile{workProfile()})

	// Decline the "save rule?" prompt — this test is only about git config values.
	out, err := executeWithInput("n\n", "use", testutil.ProfileName, "--config", cfgPath)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(out, fmt.Sprintf("Applied profile %q", testutil.ProfileName)) {
		t.Errorf("expected applied message, got: %s", out)
	}

	// Verify .git/config was updated.
	home, err := os.UserHomeDir()
	if err != nil {
		t.Skip("cannot determine home directory")
	}
	c := git.New(repoDir)
	assertGitConfig(t, c, "user.name", testutil.GitName)
	assertGitConfig(t, c, "user.email", testutil.GitEmail)
	assertGitConfig(t, c, "core.sshCommand", "ssh -i '"+filepath.Join(home, ".ssh/id_example")+"'")
	assertGitConfig(t, c, "credential.username", testutil.Username)
	assertGitConfig(t, c, "user.signingKey", testutil.SigningKey)
}

func TestUse_ProfileNotFound(t *testing.T) {
	repoDir := initGitRepo(t)
	t.Chdir(repoDir)

	cfgDir := t.TempDir()
	// Use an explicit empty config rather than relying on missing-file behaviour.
	cfgPath := writeConfig(t, cfgDir, []config.Profile{})

	_, err := execute("use", "Nonexistent", "--config", cfgPath)
	if err == nil {
		t.Fatal("expected error for non-existent profile")
	}
	if !strings.Contains(err.Error(), "not found") {
		t.Errorf("expected 'not found', got: %v", err)
	}
}

func TestUse_NotAGitRepo(t *testing.T) {
	// Use a plain temp dir with no git init.
	plainDir := t.TempDir()
	t.Chdir(plainDir)

	cfgDir := t.TempDir()
	cfgPath := writeConfig(t, cfgDir, []config.Profile{workProfile()})

	_, err := execute("use", testutil.ProfileName, "--config", cfgPath)
	if err == nil {
		t.Fatal("expected error when not in a git repo")
	}
	if !strings.Contains(err.Error(), "not a git repository") {
		t.Errorf("expected 'not a git repository', got: %v", err)
	}
}

func TestUse_IncompleteProfile_ReturnsError(t *testing.T) {
	repoDir := initGitRepo(t)
	t.Chdir(repoDir)

	cfgDir := t.TempDir()
	// Profile is missing GitName and GitEmail.
	cfgPath := writeConfig(t, cfgDir, []config.Profile{
		{Name: testutil.ProfileName},
	})

	_, err := execute("use", testutil.ProfileName, "--config", cfgPath)
	if err == nil {
		t.Fatal("expected error for incomplete profile, got nil")
	}
	if !strings.Contains(err.Error(), "incomplete") {
		t.Errorf("expected 'incomplete' in error, got: %v", err)
	}
}

func TestUse_OutputIncludesIdentity(t *testing.T) {
	repoDir := initGitRepo(t)
	t.Chdir(repoDir)

	cfgDir := t.TempDir()
	cfgPath := writeConfig(t, cfgDir, []config.Profile{workProfile()})

	// Decline the "save rule?" prompt — this test is only about output identity strings.
	out, err := executeWithInput("n\n", "use", testutil.ProfileName, "--config", cfgPath)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Output should include the git name and email.
	for _, want := range []string{testutil.GitName, testutil.GitEmail} {
		if !strings.Contains(out, want) {
			t.Errorf("expected %q in output, got: %s", want, out)
		}
	}
}

// --- smart prompt: save rule after 'gids use' ---

func TestUse_SmartPrompt_AcceptsYes_SavesRule(t *testing.T) {
	repoDir := initGitRepo(t)
	t.Chdir(repoDir)

	cfgDir := t.TempDir()
	cfgPath := writeConfig(t, cfgDir, []config.Profile{workProfile()})

	// Answer "y" to the "Always use..." prompt.
	out, err := executeWithInput("y\n", "use", testutil.ProfileName, "--config", cfgPath)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(out, "Always use") {
		t.Errorf("expected prompt in output, got: %s", out)
	}

	// Verify rule was saved.
	cfg, err := config.Load(cfgPath)
	if err != nil {
		t.Fatalf("loading config: %v", err)
	}
	_, matched := config.MatchRule(cfg.Rules, repoDir)
	if !matched {
		t.Errorf("expected rule for %q to be saved, rules: %v", repoDir, cfg.Rules)
	}
}

func TestUse_SmartPrompt_AcceptsNo_NoRuleSaved(t *testing.T) {
	repoDir := initGitRepo(t)
	t.Chdir(repoDir)

	cfgDir := t.TempDir()
	cfgPath := writeConfig(t, cfgDir, []config.Profile{workProfile()})

	// Answer "n" — no rule should be created.
	_, err := executeWithInput("n\n", "use", testutil.ProfileName, "--config", cfgPath)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	cfg, err := config.Load(cfgPath)
	if err != nil {
		t.Fatalf("loading config: %v", err)
	}
	if len(cfg.Rules) != 0 {
		t.Errorf("expected no rules after decline, got: %v", cfg.Rules)
	}
}

func TestUse_SmartPrompt_DefaultYes_SavesRule(t *testing.T) {
	repoDir := initGitRepo(t)
	t.Chdir(repoDir)

	cfgDir := t.TempDir()
	cfgPath := writeConfig(t, cfgDir, []config.Profile{workProfile()})

	// Empty input → default Y.
	_, err := executeWithInput("\n", "use", testutil.ProfileName, "--config", cfgPath)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	cfg, err := config.Load(cfgPath)
	if err != nil {
		t.Fatalf("loading config: %v", err)
	}
	_, matched := config.MatchRule(cfg.Rules, repoDir)
	if !matched {
		t.Errorf("expected rule saved with default yes, rules: %v", cfg.Rules)
	}
}

func TestUse_SmartPrompt_SecondRun_NoPrompt(t *testing.T) {
	// Verify that once a tilde-keyed rule is saved on the first run, the second
	// run correctly detects the match and skips the prompt — exercising the
	// tildify/MatchRule round-trip through expandTilde.
	repoDir := initGitRepo(t)
	t.Chdir(repoDir)

	cfgDir := t.TempDir()
	cfgPath := writeConfig(t, cfgDir, []config.Profile{workProfile()})

	// First run: accept the prompt → rule saved with tildified key.
	if _, err := executeWithInput("y\n", "use", testutil.ProfileName, "--config", cfgPath); err != nil {
		t.Fatalf("first run: %v", err)
	}

	// Second run: must not show the prompt.
	out, err := execute("use", testutil.ProfileName, "--config", cfgPath)
	if err != nil {
		t.Fatalf("second run: %v", err)
	}
	if strings.Contains(out, "Always use") {
		t.Errorf("expected no prompt on second run, got: %s", out)
	}
}

func TestUse_SmartPrompt_RuleAlreadyExists_NoPrompt(t *testing.T) {
	repoDir := initGitRepo(t)
	t.Chdir(repoDir)

	cfgDir := t.TempDir()
	cfg := &config.AppConfig{
		Profiles: []config.Profile{workProfile()},
		Rules:    map[string]string{repoDir: testutil.ProfileName},
	}
	cfgPath := writeRuleConfig(t, cfgDir, cfg)

	// Empty stdin — prompt must not appear because rule already exists.
	out, err := execute("use", testutil.ProfileName, "--config", cfgPath)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if strings.Contains(out, "Always use") {
		t.Errorf("expected no prompt when rule already exists, got: %s", out)
	}
}

// assertGitConfig reads a git config key and fails the test if it doesn't match want.
func assertGitConfig(t *testing.T, c *git.Client, key, want string) {
	t.Helper()
	got, err := c.ConfigGet(key)
	if err != nil {
		t.Fatalf("ConfigGet(%q): %v", key, err)
	}
	if got != want {
		t.Errorf("git config %s = %q, want %q", key, got, want)
	}
}
