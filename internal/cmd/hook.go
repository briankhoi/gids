package cmd

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	"gids/internal/config"
	"gids/internal/git"
)

const (
	hookBeginMarker = "# gids:hook:begin"
	hookEndMarker   = "# gids:hook:end"
)

// shellScript returns the complete shell hook script (including begin/end markers)
// for the given shell. The script calls 'gids check' on directory change.
func shellScript(shell string) (string, error) {
	var inner string
	switch shell {
	case "zsh":
		inner = `_gids_check() { gids check 2>/dev/null || true; }
autoload -Uz add-zsh-hook
add-zsh-hook chpwd _gids_check`
	case "bash":
		inner = `_gids_check() { gids check 2>/dev/null || true; }
if [[ "${PROMPT_COMMAND}" != *"_gids_check"* ]]; then
  PROMPT_COMMAND="_gids_check${PROMPT_COMMAND:+;${PROMPT_COMMAND}}"
fi`
	case "fish":
		inner = `function _gids_check --on-variable PWD
  gids check 2>/dev/null
end`
	case "powershell":
		inner = `$_gidsOrigPrompt = $function:prompt
function prompt { gids check 2>$null; & $_gidsOrigPrompt }`
	default:
		return "", fmt.Errorf("unsupported shell %q; supported: zsh, bash, fish, powershell", shell)
	}
	return hookBeginMarker + "\n" + inner + "\n" + hookEndMarker, nil
}

// defaultShellConfigPath returns the default shell config file path for shell.
func defaultShellConfigPath(shell string) (string, error) {
	home, err := homeDir()
	if err != nil {
		return "", err
	}
	switch shell {
	case "zsh":
		return filepath.Join(home, ".zshrc"), nil
	case "bash":
		return filepath.Join(home, ".bashrc"), nil
	case "fish":
		return filepath.Join(home, ".config", "fish", "config.fish"), nil
	case "powershell":
		return filepath.Join(home, ".config", "powershell", "Microsoft.PowerShell_profile.ps1"), nil
	default:
		return "", fmt.Errorf("unsupported shell %q; supported: zsh, bash, fish, powershell", shell)
	}
}

// hookInstalled reports whether the gids hook is present in content by
// checking for the begin marker only. Checking for the end marker is
// intentionally omitted: a truncated file (begin present, end missing) is
// treated as installed so that removeHook and addHook handle recovery
// gracefully rather than appending a second block.
func hookInstalled(content string) bool {
	return strings.Contains(content, hookBeginMarker)
}

// addHook appends hookScript to content. If a hook block is already present it
// is replaced. A trailing newline is ensured before appending.
func addHook(content, hookScript string) string {
	out := content
	if hookInstalled(out) {
		out = removeHook(out)
	}
	if out != "" && !strings.HasSuffix(out, "\n") {
		out += "\n"
	}
	return out + hookScript + "\n"
}

// removeHook deletes the begin…end hook block from content, including the
// trailing newline after the end marker. Returns content unchanged when no
// hook is present.
func removeHook(content string) string {
	begin := strings.Index(content, hookBeginMarker)
	if begin == -1 {
		return content
	}
	tail := content[begin:]
	endIdx := strings.Index(tail, hookEndMarker)
	if endIdx == -1 {
		return content
	}
	end := begin + endIdx + len(hookEndMarker)
	// Consume the newline that follows the end marker.
	if end < len(content) && content[end] == '\n' {
		end++
	}
	return content[:begin] + content[end:]
}

// detectShell returns the shell name derived from the $SHELL environment variable.
func detectShell() (string, error) {
	shell := os.Getenv("SHELL")
	if shell == "" {
		return "", fmt.Errorf("$SHELL is not set; specify shell with --shell")
	}
	return filepath.Base(shell), nil
}

// newHookCmd builds the 'hook' command group. When called with a shell name
// (zsh, bash, fish, powershell) it prints the hook script. The install and
// uninstall subcommands manage the hook in shell config files automatically.
func newHookCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "hook [<shell>]",
		Short: "Print or manage shell hook scripts for profile auto-switching",
		Long: `Print or manage the shell hook that applies profiles automatically when you cd.

Supported shells: zsh, bash, fish, powershell

To set up auto-switching manually, append the hook to your shell config:
  gids hook zsh  >> ~/.zshrc
  gids hook bash >> ~/.bashrc

To let gids manage the hook automatically:
  gids hook install
  gids hook uninstall`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			script, err := shellScript(args[0])
			if err != nil {
				return err
			}
			fmt.Fprintln(cmd.OutOrStdout(), script)
			return nil
		},
	}

	cmd.AddCommand(newHookInstallCmd())
	cmd.AddCommand(newHookUninstallCmd())

	return cmd
}

func newHookInstallCmd() *cobra.Command {
	var shell, file string

	cmd := &cobra.Command{
		Use:   "install",
		Short: "Install the shell hook into your shell config file",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if shell == "" {
				var err error
				shell, err = detectShell()
				if err != nil {
					return err
				}
			}

			script, err := shellScript(shell)
			if err != nil {
				return err
			}

			if file == "" {
				file, err = defaultShellConfigPath(shell)
				if err != nil {
					return err
				}
			}

			existing, err := os.ReadFile(file)
			if err != nil && !errors.Is(err, os.ErrNotExist) {
				return fmt.Errorf("reading %s: %w", tildify(file), err)
			}
			content := string(existing)

			if hookInstalled(content) {
				fmt.Fprintf(cmd.OutOrStdout(), "Hook already installed in %s.\n", tildify(file))
				return nil
			}

			if err := os.MkdirAll(filepath.Dir(file), 0o755); err != nil {
				return fmt.Errorf("creating directory: %w", err)
			}

			updated := addHook(content, script)
			if err := os.WriteFile(file, []byte(updated), 0o644); err != nil {
				return fmt.Errorf("writing %s: %w", tildify(file), err)
			}

			fmt.Fprintf(cmd.OutOrStdout(), "Hook installed in %s.\n", tildify(file))
			fmt.Fprintf(cmd.OutOrStdout(), "Restart your shell or run: source %s\n", tildify(file))
			return nil
		},
	}

	cmd.Flags().StringVar(&shell, "shell", "", "shell type (default: detect from $SHELL)")
	cmd.Flags().StringVar(&file, "file", "", "shell config file (default: auto-detect per shell)")
	return cmd
}

func newHookUninstallCmd() *cobra.Command {
	var shell, file string

	cmd := &cobra.Command{
		Use:   "uninstall",
		Short: "Remove the shell hook from your shell config file",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if shell == "" {
				var err error
				shell, err = detectShell()
				if err != nil {
					return err
				}
			}

			if file == "" {
				var err error
				file, err = defaultShellConfigPath(shell)
				if err != nil {
					return err
				}
			}

			existing, err := os.ReadFile(file)
			if err != nil {
				if errors.Is(err, os.ErrNotExist) {
					fmt.Fprintf(cmd.OutOrStdout(), "Hook not installed (%s does not exist).\n", tildify(file))
					return nil
				}
				return fmt.Errorf("reading %s: %w", tildify(file), err)
			}
			content := string(existing)

			if !hookInstalled(content) {
				fmt.Fprintf(cmd.OutOrStdout(), "Hook not installed in %s.\n", tildify(file))
				return nil
			}

			updated := removeHook(content)
			if err := os.WriteFile(file, []byte(updated), 0o644); err != nil {
				return fmt.Errorf("writing %s: %w", tildify(file), err)
			}

			fmt.Fprintf(cmd.OutOrStdout(), "Hook removed from %s.\n", tildify(file))
			return nil
		},
	}

	cmd.Flags().StringVar(&shell, "shell", "", "shell type (default: detect from $SHELL)")
	cmd.Flags().StringVar(&file, "file", "", "shell config file (default: auto-detect per shell)")
	return cmd
}

// newCheckCmd builds the 'check' command, called by the shell hook on cd.
func newCheckCmd() *cobra.Command {
	var cfgPath string

	cmd := &cobra.Command{
		Use:    "check",
		Short:  "Apply the matching profile for the current directory",
		Long:   "check is called by the shell hook on every directory change. It silently applies the matching profile when a rule matches, and does nothing otherwise.",
		Args:   cobra.NoArgs,
		Hidden: true, // not a typical user-facing command
		RunE: func(cmd *cobra.Command, args []string) error {
			cwd, err := os.Getwd()
			if err != nil {
				return fmt.Errorf("getting current directory: %w", err)
			}

			cfg, err := config.Load(cfgPath)
			if err != nil {
				return fmt.Errorf("loading config: %w", err)
			}

			_, profileName, ok := config.FindMatchingRule(cfg.Rules, cwd)
			if !ok {
				return nil // no rule matches — nothing to do
			}

			p := cfg.LookupProfile(profileName)
			if p == nil {
				return nil // rule references a deleted profile — skip silently
			}

			client := git.New(cwd)
			isRepo, err := client.IsRepo()
			if err != nil {
				return fmt.Errorf("checking git repo: %w", err)
			}
			if !isRepo {
				return nil // not a git repo — nothing to do
			}

			return git.Apply(client, *p)
		},
	}

	cmd.Flags().StringVar(&cfgPath, "config", "",
		"path to config file (default: $UserConfigDir/gids/config.yaml)")
	return cmd
}
