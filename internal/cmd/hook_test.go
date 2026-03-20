package cmd_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"gids/internal/config"
	"gids/internal/git"
	"gids/internal/testutil"
)

// isolateGitConfig redirects git global config reads/writes to a temporary file
// so tests never touch the developer's real ~/.gitconfig.
func isolateGitConfig(t *testing.T) {
	t.Helper()
	t.Setenv("GIT_CONFIG_GLOBAL", filepath.Join(t.TempDir(), "gitconfig"))
}

// --- gids hook <shell> ---

func TestHookZsh_PrintsScript(t *testing.T) {
	out, err := execute("hook", "zsh")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(out, "gids check") {
		t.Errorf("expected 'gids check' in zsh hook, got: %s", out)
	}
}

func TestHookBash_PrintsScript(t *testing.T) {
	out, err := execute("hook", "bash")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(out, "gids check") {
		t.Errorf("expected 'gids check' in bash hook, got: %s", out)
	}
}

func TestHookFish_PrintsScript(t *testing.T) {
	out, err := execute("hook", "fish")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(out, "gids check") {
		t.Errorf("expected 'gids check' in fish hook, got: %s", out)
	}
}

func TestHookUnknownShell_ReturnsError(t *testing.T) {
	_, err := execute("hook", "tcsh")
	if err == nil {
		t.Fatal("expected error for unknown shell")
	}
	if !strings.Contains(err.Error(), "unsupported") {
		t.Errorf("expected 'unsupported' in error, got: %v", err)
	}
}

func TestHookInstall_DetectsShellFromEnv(t *testing.T) {
	isolateGitConfig(t)
	t.Setenv("SHELL", "/usr/bin/zsh")
	dir := t.TempDir()
	file := filepath.Join(dir, ".zshrc")
	hooksDir := t.TempDir()

	// No --shell flag — must auto-detect from $SHELL.
	_, err := execute("hook", "install", "--file", file, "--git-hooks-dir", hooksDir)
	if err != nil {
		t.Fatalf("unexpected error with auto-detected shell: %v", err)
	}
	content, err := os.ReadFile(file)
	if err != nil {
		t.Fatalf("reading installed file: %v", err)
	}
	if !strings.Contains(string(content), "gids check") {
		t.Errorf("auto-detected install missing 'gids check': %s", string(content))
	}
}

func TestHookInstall_NoShellEnv_ReturnsError(t *testing.T) {
	t.Setenv("SHELL", "")
	dir := t.TempDir()
	file := filepath.Join(dir, ".zshrc")

	_, err := execute("hook", "install", "--file", file)
	if err == nil {
		t.Fatal("expected error when $SHELL is not set")
	}
	if !strings.Contains(err.Error(), "$SHELL is not set") {
		t.Errorf("expected '$SHELL is not set' in error, got: %v", err)
	}
}

// --- gids hook install ---

func TestHookInstall_CreatesFile(t *testing.T) {
	isolateGitConfig(t)
	dir := t.TempDir()
	file := filepath.Join(dir, ".zshrc")
	hooksDir := t.TempDir()

	out, err := execute("hook", "install", "--shell", "zsh", "--file", file, "--git-hooks-dir", hooksDir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(out, "installed") {
		t.Errorf("expected 'installed' in output, got: %s", out)
	}

	content, err := os.ReadFile(file)
	if err != nil {
		t.Fatalf("reading installed file: %v", err)
	}
	if !strings.Contains(string(content), "gids check") {
		t.Errorf("installed file missing 'gids check'")
	}
}

func TestHookInstall_PreservesExistingContent(t *testing.T) {
	isolateGitConfig(t)
	dir := t.TempDir()
	file := filepath.Join(dir, ".zshrc")
	existing := "export PATH=/usr/local/bin:$PATH\n"
	if err := os.WriteFile(file, []byte(existing), 0o644); err != nil {
		t.Fatalf("setup: %v", err)
	}
	hooksDir := t.TempDir()

	_, err := execute("hook", "install", "--shell", "zsh", "--file", file, "--git-hooks-dir", hooksDir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	content, err := os.ReadFile(file)
	if err != nil {
		t.Fatalf("reading file: %v", err)
	}
	if !strings.Contains(string(content), "PATH") {
		t.Errorf("install should preserve existing content, got: %s", string(content))
	}
	if !strings.Contains(string(content), "gids check") {
		t.Errorf("install should add gids check, got: %s", string(content))
	}
}

func TestHookInstall_Idempotent(t *testing.T) {
	isolateGitConfig(t)
	dir := t.TempDir()
	file := filepath.Join(dir, ".zshrc")
	hooksDir := t.TempDir()

	if _, err := execute("hook", "install", "--shell", "zsh", "--file", file, "--git-hooks-dir", hooksDir); err != nil {
		t.Fatalf("first install: %v", err)
	}

	out, err := execute("hook", "install", "--shell", "zsh", "--file", file, "--git-hooks-dir", hooksDir)
	if err != nil {
		t.Fatalf("second install: %v", err)
	}
	if !strings.Contains(out, "already installed") {
		t.Errorf("expected 'already installed' on second install, got: %s", out)
	}

	content, err := os.ReadFile(file)
	if err != nil {
		t.Fatalf("reading file: %v", err)
	}
	if count := strings.Count(string(content), "gids:hook:begin"); count != 1 {
		t.Errorf("expected exactly 1 hook block, found %d", count)
	}
}

// --- gids hook uninstall ---

func TestHookUninstall_RemovesBlock(t *testing.T) {
	isolateGitConfig(t)
	dir := t.TempDir()
	file := filepath.Join(dir, ".zshrc")
	hooksDir := t.TempDir()

	if _, err := execute("hook", "install", "--shell", "zsh", "--file", file, "--git-hooks-dir", hooksDir); err != nil {
		t.Fatalf("install: %v", err)
	}

	out, err := execute("hook", "uninstall", "--shell", "zsh", "--file", file)
	if err != nil {
		t.Fatalf("uninstall: %v", err)
	}
	if !strings.Contains(out, "removed") {
		t.Errorf("expected 'removed' in output, got: %s", out)
	}

	content, err := os.ReadFile(file)
	if err != nil {
		t.Fatalf("reading file: %v", err)
	}
	if strings.Contains(string(content), "gids check") {
		t.Errorf("uninstalled file still contains 'gids check': %s", string(content))
	}
}

func TestHookUninstall_NoopWhenNotInstalled(t *testing.T) {
	isolateGitConfig(t)
	dir := t.TempDir()
	file := filepath.Join(dir, ".zshrc")
	if err := os.WriteFile(file, []byte("# regular config\n"), 0o644); err != nil {
		t.Fatalf("setup: %v", err)
	}

	out, err := execute("hook", "uninstall", "--shell", "zsh", "--file", file)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(out, "not installed") {
		t.Errorf("expected 'not installed' in output, got: %s", out)
	}
}

func TestHookUninstall_MissingFile_ReportsNotInstalled(t *testing.T) {
	isolateGitConfig(t)
	dir := t.TempDir()
	file := filepath.Join(dir, "nonexistent.zshrc")

	out, err := execute("hook", "uninstall", "--shell", "zsh", "--file", file)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(out, "not installed") {
		t.Errorf("expected 'not installed' for missing file, got: %s", out)
	}
}

// --- gids check ---

func TestCheck_NoMatchingRule_Silent(t *testing.T) {
	repoDir := initGitRepo(t)
	t.Chdir(repoDir)

	cfgDir := t.TempDir()
	cfgPath := writeConfig(t, cfgDir, []config.Profile{workProfile()})

	out, err := execute("check", "--config", cfgPath)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out != "" {
		t.Errorf("expected silent output, got: %q", out)
	}
}

func TestCheck_NotInGitRepo_Silent(t *testing.T) {
	plainDir := t.TempDir()
	t.Chdir(plainDir)

	cfgDir := t.TempDir()
	cfg := &config.AppConfig{
		Profiles: []config.Profile{workProfile()},
		// Exact match on plainDir — no wildcard needed for this test.
		Rules: map[string]string{plainDir: testutil.ProfileName},
	}
	cfgPath := writeRuleConfig(t, cfgDir, cfg)

	out, err := execute("check", "--config", cfgPath)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out != "" {
		t.Errorf("expected silent output for non-git dir, got: %q", out)
	}
}

func TestCheck_AppliesMatchingProfile(t *testing.T) {
	repoDir := initGitRepo(t)
	t.Chdir(repoDir)

	cfgDir := t.TempDir()
	cfg := &config.AppConfig{
		Profiles: []config.Profile{workProfile()},
		// Exact-match glob on the repo dir itself.
		Rules: map[string]string{repoDir: testutil.ProfileName},
	}
	cfgPath := writeRuleConfig(t, cfgDir, cfg)

	out, err := execute("check", "--config", cfgPath)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out != "" {
		t.Errorf("expected silent output on success, got: %q", out)
	}

	c := git.New(repoDir)
	assertGitConfig(t, c, "user.name", testutil.GitName)
	assertGitConfig(t, c, "user.email", testutil.GitEmail)
}

func TestCheck_RulePointsToDeletedProfile_Silent(t *testing.T) {
	repoDir := initGitRepo(t)
	t.Chdir(repoDir)

	cfgDir := t.TempDir()
	cfg := &config.AppConfig{
		Profiles: []config.Profile{},
		// Rule references a profile that doesn't exist.
		Rules: map[string]string{repoDir: testutil.ProfileName},
	}
	cfgPath := writeRuleConfig(t, cfgDir, cfg)

	out, err := execute("check", "--config", cfgPath)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out != "" {
		t.Errorf("expected silent output for missing profile, got: %q", out)
	}
}

// --- git pre-commit hook (install/uninstall) ---

// TestHookInstall_WritesPreCommitScript verifies that hook install writes an
// executable pre-commit script containing "gids guard".
func TestHookInstall_WritesPreCommitScript(t *testing.T) {
	isolateGitConfig(t)
	hooksDir := t.TempDir()
	shellFile := filepath.Join(t.TempDir(), ".zshrc")

	_, err := execute("hook", "install", "--shell", "zsh", "--file", shellFile, "--git-hooks-dir", hooksDir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	hookPath := filepath.Join(hooksDir, "pre-commit")
	content, err := os.ReadFile(hookPath)
	if err != nil {
		t.Fatalf("reading pre-commit script: %v", err)
	}
	if !strings.Contains(string(content), "gids guard") {
		t.Errorf("pre-commit script missing 'gids guard', got: %s", string(content))
	}

	info, err := os.Stat(hookPath)
	if err != nil {
		t.Fatalf("stat pre-commit: %v", err)
	}
	if info.Mode()&0o100 == 0 {
		t.Errorf("pre-commit script not executable: mode %v", info.Mode())
	}
}

// TestHookInstall_SetsGlobalHooksPath verifies that hook install sets
// core.hooksPath in the global git config.
func TestHookInstall_SetsGlobalHooksPath(t *testing.T) {
	isolateGitConfig(t)
	hooksDir := t.TempDir()
	shellFile := filepath.Join(t.TempDir(), ".zshrc")

	_, err := execute("hook", "install", "--shell", "zsh", "--file", shellFile, "--git-hooks-dir", hooksDir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	got, err := git.ConfigGetGlobal("core.hooksPath")
	if err != nil {
		t.Fatalf("ConfigGetGlobal: %v", err)
	}
	if got != hooksDir {
		t.Errorf("core.hooksPath = %q, want %q", got, hooksDir)
	}
}

// TestHookInstall_GitHook_Idempotent verifies that running hook install twice
// succeeds without error and leaves exactly one pre-commit script.
func TestHookInstall_GitHook_Idempotent(t *testing.T) {
	isolateGitConfig(t)
	hooksDir := t.TempDir()
	shellFile := filepath.Join(t.TempDir(), ".zshrc")

	for i := range 2 {
		if _, err := execute("hook", "install", "--shell", "zsh", "--file", shellFile, "--git-hooks-dir", hooksDir); err != nil {
			t.Fatalf("install #%d: %v", i+1, err)
		}
	}

	entries, err := os.ReadDir(hooksDir)
	if err != nil {
		t.Fatalf("reading hooks dir: %v", err)
	}
	if len(entries) != 1 || entries[0].Name() != "pre-commit" {
		t.Errorf("expected exactly one pre-commit file, got: %v", entries)
	}
}

// TestHookUninstall_UnsetsGlobalHooksPath verifies that hook uninstall removes
// core.hooksPath from the global git config.
func TestHookUninstall_UnsetsGlobalHooksPath(t *testing.T) {
	isolateGitConfig(t)
	hooksDir := t.TempDir()
	shellFile := filepath.Join(t.TempDir(), ".zshrc")

	if _, err := execute("hook", "install", "--shell", "zsh", "--file", shellFile, "--git-hooks-dir", hooksDir); err != nil {
		t.Fatalf("install: %v", err)
	}
	if _, err := execute("hook", "uninstall", "--shell", "zsh", "--file", shellFile); err != nil {
		t.Fatalf("uninstall: %v", err)
	}

	got, err := git.ConfigGetGlobal("core.hooksPath")
	if err != nil {
		t.Fatalf("ConfigGetGlobal: %v", err)
	}
	if got != "" {
		t.Errorf("expected core.hooksPath to be unset, got %q", got)
	}
}

// TestHookUninstall_HooksPathNotSet_NoError verifies that uninstalling when the
// git hook was never installed does not return an error.
func TestHookUninstall_HooksPathNotSet_NoError(t *testing.T) {
	isolateGitConfig(t)
	shellFile := filepath.Join(t.TempDir(), ".zshrc")
	if err := os.WriteFile(shellFile, []byte("# regular config\n"), 0o644); err != nil {
		t.Fatalf("setup: %v", err)
	}

	_, err := execute("hook", "uninstall", "--shell", "zsh", "--file", shellFile)
	if err != nil {
		t.Fatalf("unexpected error when uninstalling without prior install: %v", err)
	}
}

// TestHookInstall_RunsGidGuardScript verifies the pre-commit script invokes git
// using a shell shebang so it's callable by git's hook runner.
func TestHookInstall_PreCommitScriptHasShebang(t *testing.T) {
	isolateGitConfig(t)
	hooksDir := t.TempDir()
	shellFile := filepath.Join(t.TempDir(), ".zshrc")

	_, err := execute("hook", "install", "--shell", "zsh", "--file", shellFile, "--git-hooks-dir", hooksDir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	content, err := os.ReadFile(filepath.Join(hooksDir, "pre-commit"))
	if err != nil {
		t.Fatalf("reading pre-commit: %v", err)
	}
	if !strings.HasPrefix(string(content), "#!/") {
		t.Errorf("pre-commit script missing shebang line, got: %s", string(content))
	}
}

// verifyHooksPathViaGitCmd reads core.hooksPath using a raw exec.Command to
// confirm the value is visible to the real git binary (not just via our package).
func verifyHooksPathViaGitCmd(t *testing.T, want string) {
	t.Helper()
	out, err := exec.Command("git", "config", "--global", "--get", "core.hooksPath").Output()
	if err != nil {
		if want == "" {
			return // expected not set
		}
		t.Fatalf("git config --global --get core.hooksPath: %v", err)
	}
	got := strings.TrimRight(string(out), "\n")
	if got != want {
		t.Errorf("core.hooksPath = %q, want %q", got, want)
	}
}
