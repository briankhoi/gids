package cmd

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"

	"gids/internal/config"
	"gids/internal/sshconfig"
)

func warnNoAuth(w io.Writer, username, sshKey, signingKey string) {
	if username == "" && sshKey == "" && signingKey == "" {
		fmt.Fprintln(w, "No auth method set - push/pull will use your system default.")
	}
}

// buildProfileFromPrompts collects profile fields interactively.
// Returns an error if any required field is empty or the profile name already exists in cfg.
func buildProfileFromPrompts(r *bufio.Reader, w io.Writer, cfg *config.AppConfig) (config.Profile, error) {
	name, err := prompt(r, w, "Profile name (e.g. Work, Personal): ")
	if err != nil {
		return config.Profile{}, err
	}
	if name == "" {
		return config.Profile{}, fmt.Errorf("profile name is required")
	}
	if p := cfg.LookupProfile(name); p != nil {
		return config.Profile{}, fmt.Errorf("profile %q already exists", name)
	}

	gitName, err := prompt(r, w, "Git name: ")
	if err != nil {
		return config.Profile{}, err
	}
	if gitName == "" {
		return config.Profile{}, fmt.Errorf("git name is required")
	}

	gitEmail, err := prompt(r, w, "Git email: ")
	if err != nil {
		return config.Profile{}, err
	}
	if gitEmail == "" {
		return config.Profile{}, fmt.Errorf("git email is required")
	}

	username, err := prompt(r, w, "Username (optional, for HTTPS — sets credential.username, Enter to skip): ")
	if err != nil {
		return config.Profile{}, err
	}

	sshKey, err := prompt(r, w, "SSH key path (optional, e.g. ~/.ssh/id_work, Enter to skip): ")
	if err != nil {
		return config.Profile{}, err
	}

	signingKey, err := prompt(r, w, "Signing key (optional, GPG fingerprint or SSH key path, Enter to skip): ")
	if err != nil {
		return config.Profile{}, err
	}

	warnNoAuth(w, username, sshKey, signingKey)

	return config.Profile{
		Name:       name,
		GitName:    gitName,
		GitEmail:   gitEmail,
		Username:   username,
		SSHKey:     sshKey,
		SigningKey:  signingKey,
	}, nil
}

// filterHosts returns hosts whose Pattern contains filter (case-insensitive).
// Returns all hosts unchanged when filter is empty.
func filterHosts(hosts []sshconfig.Host, filter string) []sshconfig.Host {
	if filter == "" {
		return hosts
	}
	lower := strings.ToLower(filter)
	var out []sshconfig.Host
	for _, h := range hosts {
		if strings.Contains(strings.ToLower(h.Pattern), lower) {
			out = append(out, h)
		}
	}
	return out
}

// importHost prompts for Git identity fields and appends a new profile to cfg.
// Returns (profile, true, nil) on success, (zero, false, nil) if the host is skipped.
// lastGitName is shown as a default for the name prompt; pass "" for the first host.
func importHost(r *bufio.Reader, w io.Writer, cfg *config.AppConfig, h sshconfig.Host, lastGitName string) (config.Profile, bool, error) {
	if p := cfg.LookupProfile(h.Pattern); p != nil {
		fmt.Fprintf(w, "Profile %q already exists, skipping.\n", h.Pattern)
		return config.Profile{}, false, nil
	}

	if h.IdentityFile == "" {
		fmt.Fprintf(w, "No IdentityFile set for %q — SSH key will be empty.\n", h.Pattern)
	}

	namePrompt := "Git name (required): "
	if lastGitName != "" {
		namePrompt = fmt.Sprintf("Git name [%s] (Enter to reuse, or type new): ", lastGitName)
	}
	gitName, err := promptRequired(r, w, namePrompt, lastGitName)
	if err != nil {
		return config.Profile{}, false, err
	}

	gitEmail, err := promptRequired(r, w, "Git email (required): ", "")
	if err != nil {
		return config.Profile{}, false, err
	}

	p := config.Profile{
		Name:     h.Pattern,
		GitName:  gitName,
		GitEmail: gitEmail,
		Username: h.User,
		SSHKey:   h.IdentityFile,
	}
	cfg.Profiles = append(cfg.Profiles, p)
	return p, true, nil
}

// resolveSSHConfigPath returns the SSH config path to use. If filePath is set,
// it's used directly. Otherwise, the user is prompted to pick from known paths.
func resolveSSHConfigPath(r *bufio.Reader, w io.Writer, filePath string) (string, error) {
	if filePath != "" {
		return filePath, nil
	}

	candidates, err := sshconfig.DefaultConfigPaths()
	if err != nil {
		return "", fmt.Errorf("resolving SSH config paths: %w", err)
	}
	fmt.Fprintln(w, "Known SSH config locations:")
	for i, p := range candidates {
		status := "not found"
		if _, err := os.Stat(p); err == nil {
			status = "found"
		}
		fmt.Fprintf(w, "  [%d] %-35s (%s)\n", i+1, tildify(p), status)
	}
	fmt.Fprintln(w, "  [0] Enter a custom path")
	fmt.Fprintln(w)

	// Build prompt showing the full valid range, e.g. "Enter 0–2 [default: 1]: "
	max := len(candidates)
	for {
		val, err := prompt(r, w, fmt.Sprintf("Enter 0-%d [default: 1]: ", max))
		if err != nil {
			return "", err
		}
		if val == "" {
			val = "1"
		}

		if val == "0" {
			customPath, err := promptRequired(r, w, "SSH config path: ", "")
			if err != nil {
				return "", err
			}
			return customPath, nil
		}

		// Validate and map to a candidate.
		if n, err := strconv.Atoi(val); err == nil && n >= 1 && n <= max {
			return candidates[n-1], nil
		}
		fmt.Fprintf(w, "Please enter a number between 0 and %d.\n", max)
	}
}

// selectHostsToImport asks the user which hosts to import, returning the selection.
func selectHostsToImport(r *bufio.Reader, w io.Writer, hosts []sshconfig.Host) ([]sshconfig.Host, error) {
	all, err := confirmPrompt(r, w, "Import all?", true)
	if err != nil {
		return nil, err
	}
	if all {
		return hosts, nil
	}

	var selected []sshconfig.Host
	for _, h := range hosts {
		ok, err := confirmPrompt(r, w, fmt.Sprintf("Import %q?", h.Pattern), false)
		if err != nil {
			return nil, err
		}
		if ok {
			selected = append(selected, h)
		}
	}
	return selected, nil
}

// editedProfile returns a new Profile with the provided field values applied to
// base. The Name field is preserved; no fields of base are mutated.
func editedProfile(base config.Profile, gitName, gitEmail, username, sshKey, signingKey string) config.Profile {
	return config.Profile{
		Name:       base.Name,
		GitName:    gitName,
		GitEmail:   gitEmail,
		Username:   username,
		SSHKey:     sshKey,
		SigningKey:  signingKey,
	}
}
