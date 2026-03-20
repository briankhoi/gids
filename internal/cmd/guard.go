package cmd

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"strconv"

	"github.com/spf13/cobra"

	"gids/internal/config"
	"gids/internal/git"
)

// newGuardCmd builds the hidden 'guard' command, which is invoked by the git
// pre-commit hook before each commit. It always exits 0 — it never blocks a commit.
func newGuardCmd() *cobra.Command {
	var cfgPath string

	cmd := &cobra.Command{
		Use:    "guard",
		Short:  "Pre-commit identity guard (called by the git pre-commit hook)",
		Args:   cobra.NoArgs,
		Hidden: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runGuard(cmd, cfgPath)
		},
	}

	cmd.Flags().StringVar(&cfgPath, "config", "",
		"path to config file (default: $UserConfigDir/gids/config.yaml)")
	return cmd
}

func runGuard(cmd *cobra.Command, cfgPath string) error {
	cwd, err := os.Getwd()
	if err != nil {
		return nil // fail silently — never block a commit
	}

	cfg, err := config.Load(cfgPath)
	if err != nil {
		return nil
	}

	client := git.New(cwd)
	isRepo, err := client.IsRepo()
	if err != nil || !isRepo {
		return nil
	}

	name, _ := client.ConfigGetEffective("user.name")
	email, _ := client.ConfigGetEffective("user.email")

	_, profileName, mapped := config.FindMatchingRule(cfg.Rules, cwd)
	if mapped {
		return runGuardMapped(cmd, client, cfg, cfgPath, cwd, name, email, profileName)
	}

	return runGuardUnmapped(cmd, client, cfg, cfgPath, cwd, name, email)
}

// runGuardMapped handles repos with a directory rule.
// Silent if the current identity already matches; warns and offers to fix if not.
func runGuardMapped(cmd *cobra.Command, client *git.Client, cfg *config.AppConfig, _, _, name, email, profileName string) error {
	profile := cfg.LookupProfile(profileName)
	if profile == nil {
		return nil // mapped profile no longer exists — proceed silently
	}

	if name == profile.GitName && email == profile.GitEmail {
		return nil // identity already matches — nothing to do
	}

	w := cmd.OutOrStdout()
	r := bufio.NewReader(cmd.InOrStdin())

	fmt.Fprintf(w, "⚠ Committing as %s, but this directory is mapped to %s (%s).\n",
		email, profileName, profile.GitEmail)

	fix, err := confirmPrompt(r, w, "Fix and use "+profileName+"?", true)
	if err != nil || !fix {
		return nil // EOF, I/O error, or deliberate override — proceed as-is
	}

	if err := git.Apply(client, *profile); err != nil {
		fmt.Fprintf(w, "Warning: could not apply profile: %v\n", err)
	}
	return nil
}

// runGuardUnmapped handles the unmapped-repo identity guard flow.
func runGuardUnmapped(cmd *cobra.Command, client *git.Client, cfg *config.AppConfig, cfgPath, cwd, name, email string) error {
	w := cmd.OutOrStdout()
	r := bufio.NewReader(cmd.InOrStdin())

	fmt.Fprintf(w, "Committing as %s <%s>\n", name, email)

	isYou, err := confirmPrompt(r, w, "Is this you?", true)
	if err != nil {
		return nil // EOF or I/O error — proceed silently
	}

	if isYou {
		profile := cfg.LookupProfileByIdentity(name, email)
		if profile == nil {
			return guardQuickCreateProfile(r, w, cfg, cfgPath, cwd, name, email)
		}
		return guardOfferSaveRule(r, w, cfg, cfgPath, cwd, profile.Name)
	}

	if len(cfg.Profiles) == 0 {
		return guardNoProfilesWizard(r, w, client, cfg, cfgPath, cwd, name, email)
	}
	return guardSelectProfile(r, w, client, cfg, cfgPath, cwd)
}

// guardQuickCreateProfile handles: "Is this you?" → Y, but no gids profile
// matches the current identity. It prompts only for a profile name, creates the
// profile using the current git identity, then offers to save a directory rule.
func guardQuickCreateProfile(r *bufio.Reader, w io.Writer, cfg *config.AppConfig, cfgPath, cwd, name, email string) error {
	fmt.Fprintln(w, "No profile found for this identity. Let's create one.")

	profileName, err := promptRequired(r, w, "Profile name (e.g. Work, Personal): ", "")
	if err != nil {
		return nil
	}

	p := config.Profile{
		Name:     profileName,
		GitName:  name,
		GitEmail: email,
	}
	cfg.Profiles = append(cfg.Profiles, p)
	if err := config.Save(cfg, cfgPath); err != nil {
		fmt.Fprintf(w, "Warning: could not save profile: %v\n", err)
		return nil
	}

	return guardOfferSaveRule(r, w, cfg, cfgPath, cwd, profileName)
}

// guardSelectProfile shows a numbered list of profiles and applies the one the
// user selects, then offers to save a directory rule.
func guardSelectProfile(r *bufio.Reader, w io.Writer, client *git.Client, cfg *config.AppConfig, cfgPath, cwd string) error {
	fmt.Fprintln(w, "Select a profile:")
	for i, p := range cfg.Profiles {
		fmt.Fprintf(w, "  %d) %s (%s <%s>)\n", i+1, p.Name, p.GitName, p.GitEmail)
	}

	var selected config.Profile
	for {
		val, err := prompt(r, w, fmt.Sprintf("Enter choice (1-%d): ", len(cfg.Profiles)))
		if err != nil {
			return nil
		}
		n, err := strconv.Atoi(val)
		if err != nil || n < 1 || n > len(cfg.Profiles) {
			fmt.Fprintf(w, "Please enter a number between 1 and %d.\n", len(cfg.Profiles))
			continue
		}
		selected = cfg.Profiles[n-1]
		break
	}

	if err := git.Apply(client, selected); err != nil {
		fmt.Fprintf(w, "Warning: could not apply profile: %v\n", err)
		return nil
	}

	fmt.Fprintf(w, "Switched to profile %q.\n", selected.Name)
	return guardOfferSaveRule(r, w, cfg, cfgPath, cwd, selected.Name)
}

// promptWithPrefill prompts for a required field, showing the current value as
// a default hint. An empty response keeps the current value.
func promptWithPrefill(r *bufio.Reader, w io.Writer, label, current string) (string, error) {
	p := label
	if current != "" {
		p = fmt.Sprintf("%s [%s]", label, current)
	}
	return promptRequired(r, w, p+": ", current)
}

// guardNoProfilesWizard handles the blank-slate case: no profiles exist.
// It prompts for name, email (pre-filled from the current git identity), and
// a profile name, then creates the profile, applies it, and offers a directory rule.
func guardNoProfilesWizard(r *bufio.Reader, w io.Writer, client *git.Client, cfg *config.AppConfig, cfgPath, cwd, currentName, currentEmail string) error {
	fmt.Fprintln(w, "No profiles found. Let's set one up real quick.")

	name, err := promptWithPrefill(r, w, "Name", currentName)
	if err != nil {
		return nil
	}

	email, err := promptWithPrefill(r, w, "Email", currentEmail)
	if err != nil {
		return nil
	}

	profileName, err := promptRequired(r, w, "Profile name (e.g. Work, Personal): ", "")
	if err != nil {
		return nil
	}

	p := config.Profile{
		Name:     profileName,
		GitName:  name,
		GitEmail: email,
	}
	cfg.Profiles = append(cfg.Profiles, p)
	if err := config.Save(cfg, cfgPath); err != nil {
		fmt.Fprintf(w, "Warning: could not save profile: %v\n", err)
		return nil
	}

	if err := git.Apply(client, p); err != nil {
		fmt.Fprintf(w, "Warning: could not apply profile: %v\n", err)
		return nil
	}

	fmt.Fprintf(w, "Profile %q created and applied.\n", profileName)

	guardOfferSaveRule(r, w, cfg, cfgPath, cwd, profileName) // always returns nil; error return kept for interface consistency

	fmt.Fprintln(w, "Tip: run `gids profile add` anytime to add more profiles.")
	return nil
}

// guardOfferSaveRule prompts the user to save a directory-to-profile rule.
func guardOfferSaveRule(r *bufio.Reader, w io.Writer, cfg *config.AppConfig, cfgPath, cwd, profileName string) error {
	displayDir := tildify(cwd)
	save, err := confirmPrompt(r, w, fmt.Sprintf("Save %q for %s?", profileName, displayDir), true)
	if err != nil {
		return nil
	}
	if save {
		cfg.AddRule(displayDir, profileName)
		if err := config.Save(cfg, cfgPath); err != nil {
			fmt.Fprintf(w, "Warning: could not save rule: %v\n", err)
			return nil
		}
		fmt.Fprintf(w, "Rule saved: %s → %s\n", displayDir, profileName)
	}
	return nil
}
