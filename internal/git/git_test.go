package git_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"gids/internal/config"
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

// --- Apply ---

func TestApply_BasicIdentity(t *testing.T) {
	dir := initRepo(t)
	c := git.New(dir)

	p := config.Profile{
		Name:     testutil.ProfileName,
		GitName:  testutil.GitName,
		GitEmail: testutil.GitEmail,
	}
	if err := git.Apply(c, p); err != nil {
		t.Fatalf("Apply: %v", err)
	}

	assertConfigEquals(t, c, "user.name", testutil.GitName)
	assertConfigEquals(t, c, "user.email", testutil.GitEmail)
}

func TestApply_WithSSHKey_SetsSSHCommand(t *testing.T) {
	home, err := os.UserHomeDir()
	if err != nil {
		t.Skip("cannot determine home directory")
	}
	dir := initRepo(t)
	c := git.New(dir)

	p := config.Profile{
		Name:     testutil.ProfileName,
		GitName:  testutil.GitName,
		GitEmail: testutil.GitEmail,
		SSHKey:   testutil.SSHKey,
	}
	if err := git.Apply(c, p); err != nil {
		t.Fatalf("Apply: %v", err)
	}

	expanded := filepath.Join(home, ".ssh/id_example")
	assertConfigEquals(t, c, "core.sshCommand", "ssh -i '"+expanded+"'")
}

func TestApply_WithoutSSHKey_ClearsSSHCommand(t *testing.T) {
	dir := initRepo(t)
	c := git.New(dir)

	// Pre-set a sshCommand so we can verify it gets cleared.
	if err := c.ConfigSet("core.sshCommand", "ssh -i ~/.ssh/old_key"); err != nil {
		t.Fatalf("ConfigSet pre-state: %v", err)
	}

	p := config.Profile{
		Name:     testutil.ProfileName,
		GitName:  testutil.GitName,
		GitEmail: testutil.GitEmail,
		// SSHKey intentionally empty
	}
	if err := git.Apply(c, p); err != nil {
		t.Fatalf("Apply: %v", err)
	}

	assertConfigEquals(t, c, "core.sshCommand", "")
}

func TestApply_WithUsername_SetsCredentialUsername(t *testing.T) {
	dir := initRepo(t)
	c := git.New(dir)

	p := config.Profile{
		Name:     testutil.ProfileName,
		GitName:  testutil.GitName,
		GitEmail: testutil.GitEmail,
		Username: testutil.Username,
	}
	if err := git.Apply(c, p); err != nil {
		t.Fatalf("Apply: %v", err)
	}

	assertConfigEquals(t, c, "credential.username", testutil.Username)
}

func TestApply_WithoutUsername_ClearsCredentialUsername(t *testing.T) {
	dir := initRepo(t)
	c := git.New(dir)

	if err := c.ConfigSet("credential.username", "old-user"); err != nil {
		t.Fatalf("ConfigSet pre-state: %v", err)
	}

	p := config.Profile{
		Name:     testutil.ProfileName,
		GitName:  testutil.GitName,
		GitEmail: testutil.GitEmail,
		// Username intentionally empty
	}
	if err := git.Apply(c, p); err != nil {
		t.Fatalf("Apply: %v", err)
	}

	assertConfigEquals(t, c, "credential.username", "")
}

func TestApply_WithSigningKey_SetsSigningKey(t *testing.T) {
	dir := initRepo(t)
	c := git.New(dir)

	p := config.Profile{
		Name:       testutil.ProfileName,
		GitName:    testutil.GitName,
		GitEmail:   testutil.GitEmail,
		SigningKey: testutil.SigningKey,
	}
	if err := git.Apply(c, p); err != nil {
		t.Fatalf("Apply: %v", err)
	}

	assertConfigEquals(t, c, "user.signingKey", testutil.SigningKey)
}

func TestApply_WithoutSigningKey_ClearsSigningKey(t *testing.T) {
	dir := initRepo(t)
	c := git.New(dir)

	if err := c.ConfigSet("user.signingKey", "OLD_KEY"); err != nil {
		t.Fatalf("ConfigSet pre-state: %v", err)
	}

	p := config.Profile{
		Name:     testutil.ProfileName,
		GitName:  testutil.GitName,
		GitEmail: testutil.GitEmail,
		// SigningKey intentionally empty
	}
	if err := git.Apply(c, p); err != nil {
		t.Fatalf("Apply: %v", err)
	}

	assertConfigEquals(t, c, "user.signingKey", "")
}

func TestApply_FullProfile(t *testing.T) {
	home, err := os.UserHomeDir()
	if err != nil {
		t.Skip("cannot determine home directory")
	}
	dir := initRepo(t)
	c := git.New(dir)

	p := config.Profile{
		Name:       testutil.ProfileName,
		GitName:    testutil.GitName,
		GitEmail:   testutil.GitEmail,
		Username:   testutil.Username,
		SSHKey:     testutil.SSHKey,
		SigningKey: testutil.SigningKey,
	}
	if err := git.Apply(c, p); err != nil {
		t.Fatalf("Apply: %v", err)
	}

	expanded := filepath.Join(home, ".ssh/id_example")
	assertConfigEquals(t, c, "user.name", testutil.GitName)
	assertConfigEquals(t, c, "user.email", testutil.GitEmail)
	assertConfigEquals(t, c, "core.sshCommand", "ssh -i '"+expanded+"'")
	assertConfigEquals(t, c, "credential.username", testutil.Username)
	assertConfigEquals(t, c, "user.signingKey", testutil.SigningKey)
}

// --- sshCommand quoting (shell injection safety) ---

// TestApply_SSHKey_WithSpaces verifies that an SSH key path containing spaces is
// correctly single-quoted in core.sshCommand so the shell does not split it.
func TestApply_SSHKey_WithSpaces(t *testing.T) {
	dir := initRepo(t)
	c := git.New(dir)
	keyPath := "/home/alice/.ssh/my key file"

	p := config.Profile{
		Name:     testutil.ProfileName,
		GitName:  testutil.GitName,
		GitEmail: testutil.GitEmail,
		SSHKey:   keyPath,
	}
	if err := git.Apply(c, p); err != nil {
		t.Fatalf("Apply: %v", err)
	}

	want := "ssh -i '" + keyPath + "'"
	assertConfigEquals(t, c, "core.sshCommand", want)
}

// TestApply_SSHKey_WithSingleQuote verifies that embedded single quotes in key
// paths are properly escaped to prevent shell injection.
func TestApply_SSHKey_WithSingleQuote(t *testing.T) {
	dir := initRepo(t)
	c := git.New(dir)
	keyPath := "/home/alice/.ssh/it's_a_key"

	p := config.Profile{
		Name:     testutil.ProfileName,
		GitName:  testutil.GitName,
		GitEmail: testutil.GitEmail,
		SSHKey:   keyPath,
	}
	if err := git.Apply(c, p); err != nil {
		t.Fatalf("Apply: %v", err)
	}

	want := `ssh -i '/home/alice/.ssh/it'\''s_a_key'`
	assertConfigEquals(t, c, "core.sshCommand", want)
}

// TestApply_SSHKey_TildeIsExpanded verifies that a ~ prefix is expanded to the
// absolute home directory at store time, so git can locate the key without
// relying on shell tilde expansion (which does not occur inside single quotes).
func TestApply_SSHKey_TildeIsExpanded(t *testing.T) {
	home, err := os.UserHomeDir()
	if err != nil {
		t.Skip("cannot determine home directory")
	}
	dir := initRepo(t)
	c := git.New(dir)

	p := config.Profile{
		Name:     testutil.ProfileName,
		GitName:  testutil.GitName,
		GitEmail: testutil.GitEmail,
		SSHKey:   "~/.ssh/id_work",
	}
	if err := git.Apply(c, p); err != nil {
		t.Fatalf("Apply: %v", err)
	}

	want := "ssh -i '" + filepath.Join(home, ".ssh/id_work") + "'"
	assertConfigEquals(t, c, "core.sshCommand", want)
}

// TestApply_SSHKey_Tilde verifies that the legacy testutil.SSHKey fixture
// (which uses a ~ prefix) is stored with the ~ expanded to an absolute path.
func TestApply_SSHKey_Tilde(t *testing.T) {
	home, err := os.UserHomeDir()
	if err != nil {
		t.Skip("cannot determine home directory")
	}
	dir := initRepo(t)
	c := git.New(dir)

	p := config.Profile{
		Name:     testutil.ProfileName,
		GitName:  testutil.GitName,
		GitEmail: testutil.GitEmail,
		SSHKey:   testutil.SSHKey, // "~/.ssh/id_example"
	}
	if err := git.Apply(c, p); err != nil {
		t.Fatalf("Apply: %v", err)
	}

	// ~ is expanded to the absolute home directory at store time so git can
	// locate the key; single-quoted paths inside sh -c do not tilde-expand.
	want := "ssh -i '" + filepath.Join(home, ".ssh/id_example") + "'"
	assertConfigEquals(t, c, "core.sshCommand", want)
}

// --- ConfigGetEffective ---

// initRepoUserName is the user.name value seeded by initRepo.
// Defined here so ConfigGetEffective tests stay in sync with initRepo's setup.
const initRepoUserName = "Test"

func TestConfigGetEffective_ReadsLocalValue(t *testing.T) {
	dir := initRepo(t) // seeds user.name = initRepoUserName locally
	c := git.New(dir)

	got, err := c.ConfigGetEffective("user.name")
	if err != nil {
		t.Fatalf("ConfigGetEffective: %v", err)
	}
	if got != initRepoUserName {
		t.Errorf("ConfigGetEffective(user.name) = %q, want %q", got, initRepoUserName)
	}
}

func TestConfigGetEffective_UnsetKey_ReturnsEmpty(t *testing.T) {
	dir := initRepo(t)
	c := git.New(dir)

	got, err := c.ConfigGetEffective("core.sshCommand")
	if err != nil {
		t.Fatalf("ConfigGetEffective on unset key: %v", err)
	}
	if got != "" {
		t.Errorf("expected empty string for unset key, got %q", got)
	}
}

// --- Error paths ---

// TestApply_ErrorPropagates verifies that Apply surfaces the underlying git error
// when git commands cannot run (e.g., non-existent working directory).
func TestApply_ErrorPropagates(t *testing.T) {
	c := git.New("/nonexistent/path/that/does/not/exist")
	p := config.Profile{
		Name:     testutil.ProfileName,
		GitName:  testutil.GitName,
		GitEmail: testutil.GitEmail,
	}
	if err := git.Apply(c, p); err == nil {
		t.Error("expected error for invalid git directory, got nil")
	}
}

// TestConfigSet_Error verifies that ConfigSet surfaces git errors.
func TestConfigSet_Error(t *testing.T) {
	c := git.New("/nonexistent/path/that/does/not/exist")
	if err := c.ConfigSet("user.name", "x"); err == nil {
		t.Error("expected error for invalid git directory, got nil")
	}
}

// TestConfigGet_Error verifies that ConfigGet surfaces unexpected git errors
// (i.e., errors that are not "key not found").
func TestConfigGet_Error(t *testing.T) {
	c := git.New("/nonexistent/path/that/does/not/exist")
	_, err := c.ConfigGet("user.name")
	if err == nil {
		t.Error("expected error for invalid git directory, got nil")
	}
}

// TestConfigUnset_Error verifies that ConfigUnset surfaces unexpected git errors.
func TestConfigUnset_Error(t *testing.T) {
	c := git.New("/nonexistent/path/that/does/not/exist")
	if err := c.ConfigUnset("user.name"); err == nil {
		t.Error("expected error for invalid git directory, got nil")
	}
}

// assertConfigEquals is a test helper that reads a git config key and fails if
// it does not equal want.
func assertConfigEquals(t *testing.T, c *git.Client, key, want string) {
	t.Helper()
	got, err := c.ConfigGet(key)
	if err != nil {
		t.Fatalf("ConfigGet(%q): %v", key, err)
	}
	if got != want {
		t.Errorf("%s = %q, want %q", key, got, want)
	}
}
