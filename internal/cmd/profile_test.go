package cmd_test

import (
	"bytes"
	"fmt"
	"path/filepath"
	"strings"
	"testing"

	"gids/internal/cmd"
	"gids/internal/config"
	"gids/internal/testutil"
)

// executeWithInput runs a command with simulated stdin.
func executeWithInput(input string, args ...string) (string, error) {
	buf := new(bytes.Buffer)
	root := cmd.NewRootCommand()
	root.SetOut(buf)
	root.SetErr(buf)
	root.SetIn(strings.NewReader(input))
	root.SetArgs(args)
	err := root.Execute()
	return buf.String(), err
}

// writeConfig saves profiles to a temp config file and returns the path.
func writeConfig(t *testing.T, dir string, profiles []config.Profile) string {
	t.Helper()
	path := filepath.Join(dir, "config.yaml")
	if err := config.Save(&config.AppConfig{Profiles: profiles}, path); err != nil {
		t.Fatalf("writeConfig: %v", err)
	}
	return path
}

// workProfile returns a fully-populated sample profile for tests.
func workProfile() config.Profile {
	return config.Profile{
		Name:      testutil.ProfileName,
		GitName:   testutil.GitName,
		GitEmail:  testutil.GitEmail,
		Username:  testutil.Username,
		SSHKey:    testutil.SSHKey,
		SigningKey: testutil.SigningKey,
	}
}

// --- profile list ---

func TestProfileList_NoProfiles(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")

	out, err := execute("profile", "list", "--config", path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(out, "No profiles found") {
		t.Errorf("expected 'No profiles found', got: %s", out)
	}
}

func TestProfileList_WithProfiles(t *testing.T) {
	dir := t.TempDir()
	path := writeConfig(t, dir, []config.Profile{
		workProfile(),
		{Name: testutil.ProfileName2, GitName: testutil.GitName, GitEmail: testutil.GitEmail2},
	})

	out, err := execute("profile", "list", "--config", path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	for _, want := range []string{
		testutil.ProfileName, testutil.ProfileName2,
		testutil.GitEmail, testutil.GitEmail2,
		testutil.SSHKey,
	} {
		if !strings.Contains(out, want) {
			t.Errorf("expected %q in output, got: %s", want, out)
		}
	}
}

// --- profile add ---

func TestProfileAdd_Success(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")

	input := fmt.Sprintf("%s\n%s\n%s\n%s\n%s\n\n",
		testutil.ProfileName, testutil.GitName, testutil.GitEmail, testutil.Username, testutil.SSHKey)
	out, err := executeWithInput(input, "profile", "add", "--config", path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(out, fmt.Sprintf("Profile %q added.", testutil.ProfileName)) {
		t.Errorf("expected success message, got: %s", out)
	}

	cfg, err := config.Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if len(cfg.Profiles) != 1 || cfg.Profiles[0].Name != testutil.ProfileName {
		t.Errorf("expected profile %q saved, got: %v", testutil.ProfileName, cfg.Profiles)
	}
}

func TestProfileAdd_NoAuthWarning(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")

	// all optional fields empty
	input := fmt.Sprintf("%s\n%s\n%s\n\n\n\n", testutil.ProfileName, testutil.GitName, testutil.GitEmail)
	out, err := executeWithInput(input, "profile", "add", "--config", path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(out, "No auth method set") {
		t.Errorf("expected no-auth warning, got: %s", out)
	}
}

func TestProfileAdd_DuplicateName(t *testing.T) {
	dir := t.TempDir()
	path := writeConfig(t, dir, []config.Profile{workProfile()})

	_, err := executeWithInput(testutil.ProfileName+"\n", "profile", "add", "--config", path)
	if err == nil {
		t.Fatal("expected error for duplicate name")
	}
	if !strings.Contains(err.Error(), "already exists") {
		t.Errorf("expected 'already exists', got: %v", err)
	}
}

func TestProfileAdd_EmptyName(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")

	_, err := executeWithInput("\n", "profile", "add", "--config", path)
	if err == nil {
		t.Fatal("expected error for empty name")
	}
	if !strings.Contains(err.Error(), "required") {
		t.Errorf("expected 'required', got: %v", err)
	}
}

// --- profile edit ---

func TestProfileEdit_Success(t *testing.T) {
	dir := t.TempDir()
	path := writeConfig(t, dir, []config.Profile{workProfile()})

	// keep git name (empty enter), update email, keep all optional fields
	updatedEmail := "alice-updated@example.com"
	input := fmt.Sprintf("\n%s\n\n\n\n", updatedEmail)
	out, err := executeWithInput(input, "profile", "edit", testutil.ProfileName, "--config", path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(out, fmt.Sprintf("Profile %q updated.", testutil.ProfileName)) {
		t.Errorf("expected update message, got: %s", out)
	}

	cfg, _ := config.Load(path)
	if cfg.Profiles[0].GitEmail != updatedEmail {
		t.Errorf("expected updated email %q, got: %s", updatedEmail, cfg.Profiles[0].GitEmail)
	}
	if cfg.Profiles[0].GitName != testutil.GitName {
		t.Errorf("expected git name kept as %q, got: %s", testutil.GitName, cfg.Profiles[0].GitName)
	}
}

func TestProfileEdit_ClearOptionalField(t *testing.T) {
	dir := t.TempDir()
	path := writeConfig(t, dir, []config.Profile{workProfile()})

	// keep name+email, clear all optional fields with "none"
	input := "\n\nnone\nnone\nnone\n"
	_, err := executeWithInput(input, "profile", "edit", testutil.ProfileName, "--config", path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	cfg, _ := config.Load(path)
	p := cfg.Profiles[0]
	if p.SSHKey != "" || p.Username != "" || p.SigningKey != "" {
		t.Errorf("expected all optional fields cleared, got: SSHKey=%q Username=%q SigningKey=%q",
			p.SSHKey, p.Username, p.SigningKey)
	}
}

func TestProfileEdit_NotFound(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")

	_, err := executeWithInput("\n\n\n\n\n", "profile", "edit", "Nonexistent", "--config", path)
	if err == nil {
		t.Fatal("expected error for non-existent profile")
	}
	if !strings.Contains(err.Error(), "not found") {
		t.Errorf("expected 'not found', got: %v", err)
	}
}

// --- profile delete ---

func TestProfileDelete_Force(t *testing.T) {
	dir := t.TempDir()
	path := writeConfig(t, dir, []config.Profile{workProfile()})

	out, err := execute("profile", "delete", testutil.ProfileName, "--force", "--config", path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(out, fmt.Sprintf("Profile %q deleted.", testutil.ProfileName)) {
		t.Errorf("expected delete message, got: %s", out)
	}

	cfg, _ := config.Load(path)
	if len(cfg.Profiles) != 0 {
		t.Errorf("expected 0 profiles after delete, got %d", len(cfg.Profiles))
	}
}

func TestProfileDelete_ConfirmYes(t *testing.T) {
	dir := t.TempDir()
	path := writeConfig(t, dir, []config.Profile{workProfile()})

	out, err := executeWithInput("y\n", "profile", "delete", testutil.ProfileName, "--config", path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(out, fmt.Sprintf("Profile %q deleted.", testutil.ProfileName)) {
		t.Errorf("expected delete message, got: %s", out)
	}
}

func TestProfileDelete_ConfirmNo(t *testing.T) {
	dir := t.TempDir()
	path := writeConfig(t, dir, []config.Profile{workProfile()})

	out, err := executeWithInput("n\n", "profile", "delete", testutil.ProfileName, "--config", path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(out, "Aborted.") {
		t.Errorf("expected 'Aborted.', got: %s", out)
	}

	cfg, _ := config.Load(path)
	if len(cfg.Profiles) != 1 {
		t.Errorf("expected profile to remain after abort, got %d profiles", len(cfg.Profiles))
	}
}

func TestProfileDelete_NotFound(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")

	_, err := execute("profile", "delete", "Nonexistent", "--force", "--config", path)
	if err == nil {
		t.Fatal("expected error for non-existent profile")
	}
	if !strings.Contains(err.Error(), "not found") {
		t.Errorf("expected 'not found', got: %v", err)
	}
}
