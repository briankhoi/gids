package cmd_test

import (
	"strings"
	"testing"

	"gids/internal/config"
	"gids/internal/git"
	"gids/internal/testutil"
)

// TestGuard_NotInGitRepo_Silent verifies the guard exits silently when not in a git repo.
func TestGuard_NotInGitRepo_Silent(t *testing.T) {
	plainDir := t.TempDir()
	t.Chdir(plainDir)

	cfgDir := t.TempDir()
	cfgPath := writeConfig(t, cfgDir, []config.Profile{workProfile()})

	out, err := execute("guard", "--config", cfgPath)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out != "" {
		t.Errorf("expected silent output for non-git dir, got: %q", out)
	}
}

// TestGuard_UnmappedRepo_ShowsCommittingAs verifies the "Committing as" line is shown.
func TestGuard_UnmappedRepo_ShowsCommittingAs(t *testing.T) {
	repoDir := initGitRepo(t)
	setGitIdentity(t, repoDir, testutil.GitName, testutil.GitEmail)
	t.Chdir(repoDir)

	cfgDir := t.TempDir()
	cfgPath := writeConfig(t, cfgDir, []config.Profile{workProfile()})

	// Y (is this you?) → N (save rule?)
	out, err := executeWithInput("y\nn\n", "guard", "--config", cfgPath)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	for _, want := range []string{"Committing as", testutil.GitName, testutil.GitEmail} {
		if !strings.Contains(out, want) {
			t.Errorf("expected %q in output, got: %s", want, out)
		}
	}
}

// TestGuard_UnmappedRepo_IsYou_ProfileMatches_SavesRule verifies that confirming
// identity and accepting the save offer persists a directory rule.
func TestGuard_UnmappedRepo_IsYou_ProfileMatches_SavesRule(t *testing.T) {
	repoDir := initGitRepo(t)
	setGitIdentity(t, repoDir, testutil.GitName, testutil.GitEmail)
	t.Chdir(repoDir)

	cfgDir := t.TempDir()
	cfgPath := writeConfig(t, cfgDir, []config.Profile{workProfile()})

	// Y (is this you?) → Y (save rule?)
	_, err := executeWithInput("y\ny\n", "guard", "--config", cfgPath)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	cfg, err := config.Load(cfgPath)
	if err != nil {
		t.Fatalf("loading config: %v", err)
	}
	_, matched := config.MatchRule(cfg.Rules, repoDir)
	if !matched {
		t.Errorf("expected rule saved for %q, rules: %v", repoDir, cfg.Rules)
	}
}

// TestGuard_UnmappedRepo_IsYou_ProfileMatches_NoSave verifies declining the
// save offer leaves rules unchanged.
func TestGuard_UnmappedRepo_IsYou_ProfileMatches_NoSave(t *testing.T) {
	repoDir := initGitRepo(t)
	setGitIdentity(t, repoDir, testutil.GitName, testutil.GitEmail)
	t.Chdir(repoDir)

	cfgDir := t.TempDir()
	cfgPath := writeConfig(t, cfgDir, []config.Profile{workProfile()})

	// Y (is this you?) → N (save rule?)
	_, err := executeWithInput("y\nn\n", "guard", "--config", cfgPath)
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

// TestGuard_UnmappedRepo_IsYou_NoProfileMatch_QuickCreate creates a new profile
// when the current identity does not match any existing profile.
func TestGuard_UnmappedRepo_IsYou_NoProfileMatch_QuickCreate(t *testing.T) {
	repoDir := initGitRepo(t)
	setGitIdentity(t, repoDir, testutil.GitNameUnknown, testutil.GitEmailUnknown)
	t.Chdir(repoDir)

	cfgDir := t.TempDir()
	// workProfile matches testutil.GitName/Email — GitNameUnknown won't match.
	cfgPath := writeConfig(t, cfgDir, []config.Profile{workProfile()})

	// Y (is this you?) → profile name → Y (save rule?)
	input := "y\n" + testutil.ProfileNameNew + "\ny\n"
	_, err := executeWithInput(input, "guard", "--config", cfgPath)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	cfg, err := config.Load(cfgPath)
	if err != nil {
		t.Fatalf("loading config: %v", err)
	}
	p := cfg.LookupProfile(testutil.ProfileNameNew)
	if p == nil {
		t.Fatalf("expected profile %q to be created", testutil.ProfileNameNew)
	}
	if p.GitName != testutil.GitNameUnknown {
		t.Errorf("GitName = %q, want %q", p.GitName, testutil.GitNameUnknown)
	}
	if p.GitEmail != testutil.GitEmailUnknown {
		t.Errorf("GitEmail = %q, want %q", p.GitEmail, testutil.GitEmailUnknown)
	}
	_, matched := config.MatchRule(cfg.Rules, repoDir)
	if !matched {
		t.Errorf("expected rule saved for %q, rules: %v", repoDir, cfg.Rules)
	}
}

// TestGuard_UnmappedRepo_IsYou_NoProfileMatch_QuickCreate_NoSave verifies
// declining the save offer after quick-create leaves rules empty.
func TestGuard_UnmappedRepo_IsYou_NoProfileMatch_QuickCreate_NoSave(t *testing.T) {
	repoDir := initGitRepo(t)
	setGitIdentity(t, repoDir, testutil.GitNameUnknown, testutil.GitEmailUnknown)
	t.Chdir(repoDir)

	cfgDir := t.TempDir()
	cfgPath := writeConfig(t, cfgDir, []config.Profile{workProfile()})

	// Y → profile name → N (no save rule)
	input := "y\n" + testutil.ProfileNameNew + "\nn\n"
	_, err := executeWithInput(input, "guard", "--config", cfgPath)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	cfg, err := config.Load(cfgPath)
	if err != nil {
		t.Fatalf("loading config: %v", err)
	}
	if len(cfg.Rules) != 0 {
		t.Errorf("expected no rules, got: %v", cfg.Rules)
	}
	// Profile should still have been created.
	if p := cfg.LookupProfile(testutil.ProfileNameNew); p == nil {
		t.Errorf("expected profile %q to be created even without a rule", testutil.ProfileNameNew)
	}
}

// TestGuard_UnmappedRepo_NotYou_SelectProfile_Applies verifies that selecting
// a profile from the list applies it to the local git config.
func TestGuard_UnmappedRepo_NotYou_SelectProfile_Applies(t *testing.T) {
	repoDir := initGitRepo(t)
	setGitIdentity(t, repoDir, testutil.GitNameUnknown, testutil.GitEmailUnknown)
	t.Chdir(repoDir)

	cfgDir := t.TempDir()
	cfgPath := writeConfig(t, cfgDir, []config.Profile{workProfile()})

	// N (not me) → 1 (select Work) → N (don't save rule)
	_, err := executeWithInput("n\n1\nn\n", "guard", "--config", cfgPath)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	c := git.New(repoDir)
	assertGitConfig(t, c, "user.name", testutil.GitName)
	assertGitConfig(t, c, "user.email", testutil.GitEmail)
}

// TestGuard_UnmappedRepo_NotYou_SelectProfile_SavesRule verifies that selecting
// a profile and accepting the save offer creates a directory rule.
func TestGuard_UnmappedRepo_NotYou_SelectProfile_SavesRule(t *testing.T) {
	repoDir := initGitRepo(t)
	setGitIdentity(t, repoDir, testutil.GitNameUnknown, testutil.GitEmailUnknown)
	t.Chdir(repoDir)

	cfgDir := t.TempDir()
	cfgPath := writeConfig(t, cfgDir, []config.Profile{workProfile()})

	// N → 1 (Work) → Y (save rule)
	_, err := executeWithInput("n\n1\ny\n", "guard", "--config", cfgPath)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	cfg, err := config.Load(cfgPath)
	if err != nil {
		t.Fatalf("loading config: %v", err)
	}
	_, matched := config.MatchRule(cfg.Rules, repoDir)
	if !matched {
		t.Errorf("expected rule saved for %q, rules: %v", repoDir, cfg.Rules)
	}
}

// TestGuard_UnmappedRepo_NotYou_SelectProfile_InvalidThenValid verifies
// out-of-range input re-prompts until a valid selection is made.
func TestGuard_UnmappedRepo_NotYou_SelectProfile_InvalidThenValid(t *testing.T) {
	repoDir := initGitRepo(t)
	setGitIdentity(t, repoDir, testutil.GitNameUnknown, testutil.GitEmailUnknown)
	t.Chdir(repoDir)

	cfgDir := t.TempDir()
	cfgPath := writeConfig(t, cfgDir, []config.Profile{workProfile()})

	// N → 0 (invalid) → 999 (invalid) → 1 (valid Work) → N (no save)
	out, err := executeWithInput("n\n0\n999\n1\nn\n", "guard", "--config", cfgPath)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(out, "between") {
		t.Errorf("expected re-prompt error in output, got: %s", out)
	}

	c := git.New(repoDir)
	assertGitConfig(t, c, "user.name", testutil.GitName)
}

// TestGuard_UnmappedRepo_NotYou_NoProfiles_Wizard verifies the inline profile
// creation wizard for blank-slate users, including profile creation, application,
// rule saving, and the tip message.
func TestGuard_UnmappedRepo_NotYou_NoProfiles_Wizard(t *testing.T) {
	repoDir := initGitRepo(t)
	setGitIdentity(t, repoDir, testutil.GitNameUnknown, testutil.GitEmailUnknown)
	t.Chdir(repoDir)

	cfgDir := t.TempDir()
	cfgPath := writeConfig(t, cfgDir, []config.Profile{})

	// N (not me) → custom name → custom email → profile name → Y (save rule)
	const wizardProfile = "MyProfile"
	input := "n\n" + testutil.GitNameNew + "\n" + testutil.GitEmailNew + "\n" + wizardProfile + "\ny\n"
	out, err := executeWithInput(input, "guard", "--config", cfgPath)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	cfg, err := config.Load(cfgPath)
	if err != nil {
		t.Fatalf("loading config: %v", err)
	}

	p := cfg.LookupProfile(wizardProfile)
	if p == nil {
		t.Fatalf("expected profile %q to be created", wizardProfile)
	}
	if p.GitName != testutil.GitNameNew {
		t.Errorf("GitName = %q, want %q", p.GitName, testutil.GitNameNew)
	}
	if p.GitEmail != testutil.GitEmailNew {
		t.Errorf("GitEmail = %q, want %q", p.GitEmail, testutil.GitEmailNew)
	}

	_, matched := config.MatchRule(cfg.Rules, repoDir)
	if !matched {
		t.Errorf("expected rule saved for %q, rules: %v", repoDir, cfg.Rules)
	}

	// Profile should be applied to local git config.
	c := git.New(repoDir)
	assertGitConfig(t, c, "user.name", testutil.GitNameNew)
	assertGitConfig(t, c, "user.email", testutil.GitEmailNew)

	// Tip should appear in output.
	if !strings.Contains(out, "gids profile add") {
		t.Errorf("expected 'gids profile add' tip in output, got: %s", out)
	}
}

// TestGuard_UnmappedRepo_NotYou_NoProfiles_Wizard_DefaultPrefill verifies
// pressing Enter at name/email prompts accepts the pre-filled git identity.
func TestGuard_UnmappedRepo_NotYou_NoProfiles_Wizard_DefaultPrefill(t *testing.T) {
	repoDir := initGitRepo(t)
	setGitIdentity(t, repoDir, testutil.GitNameUnknown, testutil.GitEmailUnknown)
	t.Chdir(repoDir)

	cfgDir := t.TempDir()
	cfgPath := writeConfig(t, cfgDir, []config.Profile{})

	// N → Enter (keep GitNameUnknown) → Enter (keep GitEmailUnknown) → profile name → N
	const prefillProfile = "DemoProfile"
	input := "n\n\n\n" + prefillProfile + "\nn\n"
	_, err := executeWithInput(input, "guard", "--config", cfgPath)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	cfg, err := config.Load(cfgPath)
	if err != nil {
		t.Fatalf("loading config: %v", err)
	}
	p := cfg.LookupProfile(prefillProfile)
	if p == nil {
		t.Fatalf("expected profile %q to be created", prefillProfile)
	}
	if p.GitName != testutil.GitNameUnknown {
		t.Errorf("GitName = %q, want %q (pre-filled default)", p.GitName, testutil.GitNameUnknown)
	}
	if p.GitEmail != testutil.GitEmailUnknown {
		t.Errorf("GitEmail = %q, want %q (pre-filled default)", p.GitEmail, testutil.GitEmailUnknown)
	}
}

// TestGuard_MappedRepo_Silent verifies the guard exits silently when the
// current identity already matches the mapped profile.
func TestGuard_MappedRepo_Silent(t *testing.T) {
	repoDir := initGitRepo(t)
	setGitIdentity(t, repoDir, testutil.GitName, testutil.GitEmail)
	t.Chdir(repoDir)

	cfgDir := t.TempDir()
	cfg := &config.AppConfig{
		Profiles: []config.Profile{workProfile()},
		Rules:    map[string]string{repoDir: testutil.ProfileName},
	}
	cfgPath := writeRuleConfig(t, cfgDir, cfg)

	out, err := execute("guard", "--config", cfgPath)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out != "" {
		t.Errorf("expected silent output for mapped repo, got: %q", out)
	}
}

// TestGuard_MappedRepo_IdentityMismatch_FixAccepted verifies that when the
// current identity doesn't match the mapped profile and the user accepts the
// fix offer, the mapped profile is applied to local git config.
func TestGuard_MappedRepo_IdentityMismatch_FixAccepted(t *testing.T) {
	repoDir := initGitRepo(t)
	setGitIdentity(t, repoDir, testutil.GitNameUnknown, testutil.GitEmailUnknown)
	t.Chdir(repoDir)

	cfgDir := t.TempDir()
	cfg := &config.AppConfig{
		Profiles: []config.Profile{workProfile()},
		Rules:    map[string]string{repoDir: testutil.ProfileName},
	}
	cfgPath := writeRuleConfig(t, cfgDir, cfg)

	// Y → fix and apply mapped profile
	out, err := executeWithInput("y\n", "guard", "--config", cfgPath)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Warning should mention the current email and the mapped profile.
	for _, want := range []string{testutil.GitEmailUnknown, testutil.ProfileName, testutil.GitEmail} {
		if !strings.Contains(out, want) {
			t.Errorf("expected %q in mismatch warning, got: %s", want, out)
		}
	}

	// Profile should be applied.
	c := git.New(repoDir)
	assertGitConfig(t, c, "user.name", testutil.GitName)
	assertGitConfig(t, c, "user.email", testutil.GitEmail)
}

// TestGuard_MappedRepo_IdentityMismatch_FixDeclined verifies that when the
// user declines the fix offer, git config is left unchanged.
func TestGuard_MappedRepo_IdentityMismatch_FixDeclined(t *testing.T) {
	repoDir := initGitRepo(t)
	setGitIdentity(t, repoDir, testutil.GitNameUnknown, testutil.GitEmailUnknown)
	t.Chdir(repoDir)

	cfgDir := t.TempDir()
	cfg := &config.AppConfig{
		Profiles: []config.Profile{workProfile()},
		Rules:    map[string]string{repoDir: testutil.ProfileName},
	}
	cfgPath := writeRuleConfig(t, cfgDir, cfg)

	// N → proceed as-is
	_, err := executeWithInput("n\n", "guard", "--config", cfgPath)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Identity should be unchanged.
	c := git.New(repoDir)
	assertGitConfig(t, c, "user.name", testutil.GitNameUnknown)
	assertGitConfig(t, c, "user.email", testutil.GitEmailUnknown)
}
