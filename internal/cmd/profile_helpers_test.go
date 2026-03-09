package cmd

import (
	"bufio"
	"strings"
	"testing"

	"gids/internal/config"
	"gids/internal/sshconfig"
	"gids/internal/testutil"
)

// updatedGitName and updatedGitEmail are the "after-edit" values used in editedProfile tests.
const (
	updatedGitName  = "Alice Updated"
	updatedGitEmail = "alice-updated@example.com"
	updatedUsername = "alice-updated"
	updatedSSHKey   = "~/.ssh/id_updated"
	updatedSigningKey = "UPDATED1234567890"
)

// baseProfile returns a fully-populated profile using shared testutil fixtures.
func baseProfile() config.Profile {
	return config.Profile{
		Name:       testutil.ProfileName,
		GitName:    testutil.GitName,
		GitEmail:   testutil.GitEmail,
		Username:   testutil.Username,
		SSHKey:     testutil.SSHKey,
		SigningKey:  testutil.SigningKey,
	}
}

// --- buildProfileFromPrompts ---

func TestBuildProfileFromPrompts_HappyPath(t *testing.T) {
	cfg := &config.AppConfig{}
	input := strings.Join([]string{
		testutil.ProfileName,
		testutil.GitName,
		testutil.GitEmail,
		testutil.Username,
		testutil.SSHKey,
		testutil.SigningKey,
		"", // trailing newline
	}, "\n")
	r := bufio.NewReader(strings.NewReader(input))
	var w strings.Builder

	p, err := buildProfileFromPrompts(r, &w, cfg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if p.Name != testutil.ProfileName {
		t.Errorf("Name = %q, want %q", p.Name, testutil.ProfileName)
	}
	if p.GitName != testutil.GitName {
		t.Errorf("GitName = %q, want %q", p.GitName, testutil.GitName)
	}
	if p.GitEmail != testutil.GitEmail {
		t.Errorf("GitEmail = %q, want %q", p.GitEmail, testutil.GitEmail)
	}
	if p.Username != testutil.Username {
		t.Errorf("Username = %q, want %q", p.Username, testutil.Username)
	}
	if p.SSHKey != testutil.SSHKey {
		t.Errorf("SSHKey = %q, want %q", p.SSHKey, testutil.SSHKey)
	}
	if p.SigningKey != testutil.SigningKey {
		t.Errorf("SigningKey = %q, want %q", p.SigningKey, testutil.SigningKey)
	}
}

func TestBuildProfileFromPrompts_EmptyNameReturnsError(t *testing.T) {
	cfg := &config.AppConfig{}
	r := bufio.NewReader(strings.NewReader("\n"))
	var w strings.Builder

	_, err := buildProfileFromPrompts(r, &w, cfg)
	if err == nil {
		t.Fatal("expected error for empty name")
	}
	if !strings.Contains(err.Error(), "required") {
		t.Errorf("expected 'required' in error, got: %v", err)
	}
}

func TestBuildProfileFromPrompts_DuplicateNameReturnsError(t *testing.T) {
	cfg := &config.AppConfig{Profiles: []config.Profile{
		{Name: testutil.ProfileName, GitName: testutil.GitName, GitEmail: testutil.GitEmail},
	}}
	r := bufio.NewReader(strings.NewReader(testutil.ProfileName + "\n"))
	var w strings.Builder

	_, err := buildProfileFromPrompts(r, &w, cfg)
	if err == nil {
		t.Fatal("expected error for duplicate name")
	}
	if !strings.Contains(err.Error(), "already exists") {
		t.Errorf("expected 'already exists' in error, got: %v", err)
	}
}

func TestBuildProfileFromPrompts_NoAuthWarning(t *testing.T) {
	cfg := &config.AppConfig{}
	// Skip all optional fields
	input := strings.Join([]string{
		testutil.ProfileName, testutil.GitName, testutil.GitEmail,
		"", "", "", // empty optional fields
	}, "\n")
	r := bufio.NewReader(strings.NewReader(input))
	var w strings.Builder

	_, err := buildProfileFromPrompts(r, &w, cfg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(w.String(), "No auth method set") {
		t.Errorf("expected no-auth warning in output, got: %s", w.String())
	}
}

// --- filterHosts ---

func TestFilterHosts_EmptyFilterReturnsAll(t *testing.T) {
	hosts := []sshconfig.Host{
		{Pattern: testutil.SSHHostWork},
		{Pattern: testutil.SSHHostPersonal},
	}
	got := filterHosts(hosts, "")
	if len(got) != 2 {
		t.Errorf("len = %d, want 2", len(got))
	}
}

func TestFilterHosts_CaseInsensitiveMatch(t *testing.T) {
	hosts := []sshconfig.Host{
		{Pattern: testutil.SSHHostWork},
		{Pattern: testutil.SSHHostPersonal},
	}
	got := filterHosts(hosts, "WORK")
	if len(got) != 1 || got[0].Pattern != testutil.SSHHostWork {
		t.Errorf("got %v, want [{Pattern:%s}]", got, testutil.SSHHostWork)
	}
}

func TestFilterHosts_NoMatchReturnsEmpty(t *testing.T) {
	hosts := []sshconfig.Host{{Pattern: testutil.SSHHostWork}}
	got := filterHosts(hosts, "zzz")
	if len(got) != 0 {
		t.Errorf("expected empty, got %v", got)
	}
}

// --- importHost ---

func TestImportHost_HappyPath(t *testing.T) {
	cfg := &config.AppConfig{}
	h := sshconfig.Host{
		Pattern:      testutil.SSHHostWork,
		IdentityFile: testutil.SSHKey,
		User:         testutil.SSHUser,
	}
	input := testutil.GitName + "\n" + testutil.GitEmail + "\n"
	r := bufio.NewReader(strings.NewReader(input))
	var w strings.Builder

	p, imported, err := importHost(r, &w, cfg, h, "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !imported {
		t.Fatal("expected imported=true")
	}
	if p.Name != testutil.SSHHostWork {
		t.Errorf("Name = %q, want %q", p.Name, testutil.SSHHostWork)
	}
	if p.GitName != testutil.GitName {
		t.Errorf("GitName = %q, want %q", p.GitName, testutil.GitName)
	}
	if p.SSHKey != testutil.SSHKey {
		t.Errorf("SSHKey = %q, want %q", p.SSHKey, testutil.SSHKey)
	}
	if p.Username != testutil.SSHUser {
		t.Errorf("Username = %q, want %q", p.Username, testutil.SSHUser)
	}
}

func TestImportHost_SkipsExistingProfile(t *testing.T) {
	cfg := &config.AppConfig{Profiles: []config.Profile{
		{Name: testutil.SSHHostWork, GitName: testutil.GitName, GitEmail: testutil.GitEmail},
	}}
	h := sshconfig.Host{Pattern: testutil.SSHHostWork}
	r := bufio.NewReader(strings.NewReader(""))
	var w strings.Builder

	_, imported, err := importHost(r, &w, cfg, h, "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if imported {
		t.Fatal("expected imported=false for existing profile")
	}
	if !strings.Contains(w.String(), "already exists") {
		t.Errorf("expected 'already exists' in output, got: %s", w.String())
	}
}

func TestImportHost_ReusesLastGitName(t *testing.T) {
	cfg := &config.AppConfig{}
	h := sshconfig.Host{Pattern: testutil.SSHHostPersonal}
	// Empty input for name (reuse lastGitName), then email
	input := "\n" + testutil.GitEmail + "\n"
	r := bufio.NewReader(strings.NewReader(input))
	var w strings.Builder

	p, _, err := importHost(r, &w, cfg, h, testutil.GitName)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if p.GitName != testutil.GitName {
		t.Errorf("GitName = %q, want %q (reused)", p.GitName, testutil.GitName)
	}
}

func TestImportHost_WarnsOnMissingIdentityFile(t *testing.T) {
	cfg := &config.AppConfig{}
	h := sshconfig.Host{Pattern: testutil.SSHHostWork, IdentityFile: ""}
	input := testutil.GitName + "\n" + testutil.GitEmail + "\n"
	r := bufio.NewReader(strings.NewReader(input))
	var w strings.Builder

	_, _, err := importHost(r, &w, cfg, h, "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(w.String(), "SSH key will be empty") {
		t.Errorf("expected 'SSH key will be empty' warning, got: %s", w.String())
	}
}

// --- resolveSSHConfigPath ---

// candidatePaths returns the first N default SSH config candidate paths for testing.
// We query DefaultConfigPaths() here so tests stay correct across platforms.
func candidatePaths(t *testing.T) []string {
	t.Helper()
	paths, err := sshconfig.DefaultConfigPaths()
	if err != nil {
		t.Fatalf("DefaultConfigPaths: %v", err)
	}
	return paths
}

func TestResolveSSHConfigPath_ShortCircuitsWhenFileProvided(t *testing.T) {
	r := bufio.NewReader(strings.NewReader(""))
	var w strings.Builder

	got, err := resolveSSHConfigPath(r, &w, "/explicit/path")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "/explicit/path" {
		t.Errorf("got %q, want %q", got, "/explicit/path")
	}
}

func TestResolveSSHConfigPath_EmptyInputDefaultsToFirst(t *testing.T) {
	candidates := candidatePaths(t)
	r := bufio.NewReader(strings.NewReader("\n")) // empty → default 1
	var w strings.Builder

	got, err := resolveSSHConfigPath(r, &w, "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != candidates[0] {
		t.Errorf("got %q, want candidates[0] %q", got, candidates[0])
	}
}

func TestResolveSSHConfigPath_NumericSelection(t *testing.T) {
	candidates := candidatePaths(t)
	if len(candidates) < 2 {
		t.Skip("need at least 2 candidates for this test")
	}
	r := bufio.NewReader(strings.NewReader("2\n"))
	var w strings.Builder

	got, err := resolveSSHConfigPath(r, &w, "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != candidates[1] {
		t.Errorf("got %q, want candidates[1] %q", got, candidates[1])
	}
}

func TestResolveSSHConfigPath_ZeroSelectsCustomPath(t *testing.T) {
	// "0" → custom path prompt, then the custom path
	r := bufio.NewReader(strings.NewReader("0\n/my/custom/ssh_config\n"))
	var w strings.Builder

	got, err := resolveSSHConfigPath(r, &w, "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "/my/custom/ssh_config" {
		t.Errorf("got %q, want %q", got, "/my/custom/ssh_config")
	}
}

func TestResolveSSHConfigPath_OutOfRangeReprompts(t *testing.T) {
	candidates := candidatePaths(t)
	// Send an out-of-range value first, then a valid "1"
	input := "999\n1\n"
	r := bufio.NewReader(strings.NewReader(input))
	var w strings.Builder

	got, err := resolveSSHConfigPath(r, &w, "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != candidates[0] {
		t.Errorf("got %q, want candidates[0] %q", got, candidates[0])
	}
	if !strings.Contains(w.String(), "Please enter a number between") {
		t.Errorf("expected re-prompt message, got: %s", w.String())
	}
}

// --- editedProfile ---

// TestEditedProfile_AppliesAllFields verifies that editedProfile returns a new
// Profile with all provided field values applied.
func TestEditedProfile_AppliesAllFields(t *testing.T) {
	base := baseProfile()

	got := editedProfile(base, updatedGitName, updatedGitEmail, updatedUsername, updatedSSHKey, updatedSigningKey)

	cases := []struct {
		field string
		got   string
		want  string
	}{
		{"Name", got.Name, testutil.ProfileName},
		{"GitName", got.GitName, updatedGitName},
		{"GitEmail", got.GitEmail, updatedGitEmail},
		{"Username", got.Username, updatedUsername},
		{"SSHKey", got.SSHKey, updatedSSHKey},
		{"SigningKey", got.SigningKey, updatedSigningKey},
	}
	for _, tc := range cases {
		if tc.got != tc.want {
			t.Errorf("%s = %q, want %q", tc.field, tc.got, tc.want)
		}
	}
}

// TestEditedProfile_DoesNotMutateBase verifies that editedProfile never modifies
// the original profile — the base value must be unchanged after the call.
func TestEditedProfile_DoesNotMutateBase(t *testing.T) {
	base := baseProfile()

	_ = editedProfile(base, updatedGitName, updatedGitEmail, updatedUsername, updatedSSHKey, updatedSigningKey)

	if base.GitName != testutil.GitName {
		t.Errorf("base.GitName was mutated: got %q, want %q", base.GitName, testutil.GitName)
	}
	if base.GitEmail != testutil.GitEmail {
		t.Errorf("base.GitEmail was mutated: got %q, want %q", base.GitEmail, testutil.GitEmail)
	}
}
