package git_test

import (
	"os/exec"
	"testing"

	"gids/internal/git"
	"gids/internal/testutil"
)

// initRepo creates a temporary directory and initialises a bare-minimum git
// repository inside it. Returns the repo root path.
func initRepo(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	cmd := exec.Command("git", "init", dir)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git init: %v\n%s", err, out)
	}
	// Set required user config so later git commands don't fail on some systems.
	for _, kv := range [][2]string{
		{"user.email", "test@test.com"},
		{"user.name", "Test"},
	} {
		c := exec.Command("git", "-C", dir, "config", "--local", kv[0], kv[1])
		if out, err := c.CombinedOutput(); err != nil {
			t.Fatalf("git config %s: %v\n%s", kv[0], err, out)
		}
	}
	return dir
}

// --- IsRepo ---

func TestIsRepo_InRepo(t *testing.T) {
	dir := initRepo(t)
	c := git.New(dir)
	ok, err := c.IsRepo()
	if err != nil {
		t.Fatalf("IsRepo: %v", err)
	}
	if !ok {
		t.Error("expected IsRepo=true for git-init'd directory")
	}
}

func TestIsRepo_NotRepo(t *testing.T) {
	dir := t.TempDir() // plain directory, no git init
	c := git.New(dir)
	ok, err := c.IsRepo()
	if err != nil {
		t.Fatalf("IsRepo: %v", err)
	}
	if ok {
		t.Error("expected IsRepo=false for plain directory")
	}
}

// --- ConfigSet / ConfigGet ---

func TestConfigSet_And_Get(t *testing.T) {
	dir := initRepo(t)
	c := git.New(dir)

	if err := c.ConfigSet("user.name", testutil.GitName); err != nil {
		t.Fatalf("ConfigSet: %v", err)
	}

	got, err := c.ConfigGet("user.name")
	if err != nil {
		t.Fatalf("ConfigGet: %v", err)
	}
	if got != testutil.GitName {
		t.Errorf("user.name = %q, want %q", got, testutil.GitName)
	}
}

func TestConfigGet_UnsetKey_ReturnsEmpty(t *testing.T) {
	dir := initRepo(t)
	c := git.New(dir)

	got, err := c.ConfigGet("core.sshCommand")
	if err != nil {
		t.Fatalf("ConfigGet on unset key: %v", err)
	}
	if got != "" {
		t.Errorf("expected empty string for unset key, got %q", got)
	}
}

// --- ConfigUnset ---

func TestConfigUnset_ExistingKey(t *testing.T) {
	dir := initRepo(t)
	c := git.New(dir)

	if err := c.ConfigSet("core.sshCommand", "ssh -i ~/.ssh/id_work"); err != nil {
		t.Fatalf("ConfigSet: %v", err)
	}
	if err := c.ConfigUnset("core.sshCommand"); err != nil {
		t.Fatalf("ConfigUnset: %v", err)
	}

	got, err := c.ConfigGet("core.sshCommand")
	if err != nil {
		t.Fatalf("ConfigGet after unset: %v", err)
	}
	if got != "" {
		t.Errorf("expected key removed, got %q", got)
	}
}

func TestConfigUnset_MissingKey_NoError(t *testing.T) {
	dir := initRepo(t)
	c := git.New(dir)

	// Unsetting a key that was never set must not return an error.
	if err := c.ConfigUnset("core.sshCommand"); err != nil {
		t.Errorf("ConfigUnset on missing key: %v", err)
	}
}
