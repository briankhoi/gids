// Package git runs the git CLI. gids never parses porcelain meant for humans;
// it uses plumbing and `git config` whose output formats are stable.
package git

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
)

// Client runs git in Dir with extra environment variables.
type Client struct {
	Dir string
	Env []string // appended to the process environment, e.g. "GIDS_SKIP_HOOKS=1"
}

// Error is a failed git invocation.
type Error struct {
	Args   []string
	Code   int
	Stderr string
}

func (e *Error) Error() string {
	msg := strings.TrimSpace(e.Stderr)
	if msg == "" {
		msg = fmt.Sprintf("exit status %d", e.Code)
	}
	return fmt.Sprintf("git %s: %s", strings.Join(e.Args, " "), msg)
}

// ExitCode returns git's exit code for err, or -1 if err isn't a git error.
func ExitCode(err error) int {
	if ge, ok := errors.AsType[*Error](err); ok {
		return ge.Code
	}
	return -1
}

// Run runs git and returns stdout with one trailing newline removed.
func (c Client) Run(args ...string) (string, error) { return c.RunInput("", args...) }

// RunInput is Run with stdin.
func (c Client) RunInput(stdin string, args ...string) (string, error) {
	out, err := c.exec(stdin, args...)
	return strings.TrimSuffix(out, "\n"), err
}

// RunRaw is RunInput without trimming: stdout exactly as git wrote it.
func (c Client) RunRaw(stdin string, args ...string) (string, error) { return c.exec(stdin, args...) }

// exec runs git and returns stdout exactly as written.
func (c Client) exec(stdin string, args ...string) (string, error) {
	cmd := exec.Command("git", args...)
	cmd.Dir = c.Dir
	cmd.Env = append(os.Environ(), c.Env...)
	cmd.Stdin = strings.NewReader(stdin)
	var out, errb bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &errb
	if err := cmd.Run(); err != nil {
		if ee, ok := errors.AsType[*exec.ExitError](err); ok {
			return out.String(), &Error{Args: args, Code: ee.ExitCode(), Stderr: errb.String()}
		}
		if errors.Is(err, exec.ErrNotFound) {
			return "", errors.New("git isn't installed (or isn't on your PATH)")
		}
		return "", err
	}
	return out.String(), nil
}

// ConfigGet reads one config value with git's normal precedence (scope args
// like "--local" or "--global" narrow it). ok is false when the key is unset.
func (c Client) ConfigGet(key string, scope ...string) (val string, ok bool, err error) {
	args := append([]string{"config"}, scope...)
	val, err = c.Run(append(args, "--get", key)...)
	if ExitCode(err) == 1 {
		return "", false, nil
	}
	return val, err == nil, err
}

// ConfigGetAll reads every value of a multi-valued key, in config order.
func (c Client) ConfigGetAll(key string, scope ...string) ([]string, error) {
	args := append([]string{"config"}, scope...)
	val, err := c.Run(append(args, "--get-all", key)...)
	if ExitCode(err) == 1 {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return strings.Split(val, "\n"), nil
}

// Source is where git found a config value.
type Source struct {
	Scope  string // "local", "global", "system", "worktree", "command"
	Origin string // as --show-origin prints it, e.g. "file:/home/x/.gitconfig"
	Value  string
}

// ConfigSource returns the effective value of key and where it came from.
func (c Client) ConfigSource(key string) (src Source, ok bool, err error) {
	out, err := c.Run("config", "--show-scope", "--show-origin", "--get", key)
	if ExitCode(err) == 1 {
		return Source{}, false, nil
	}
	if err != nil {
		return Source{}, false, err
	}
	parts := strings.SplitN(out, "\t", 3)
	if len(parts) != 3 {
		return Source{}, false, fmt.Errorf("unexpected git config output %q", out)
	}
	return Source{Scope: parts[0], Origin: parts[1], Value: parts[2]}, true, nil
}

// ConfigUnsetLocal removes key from the repo's .git/config. Unset is fine.
func (c Client) ConfigUnsetLocal(key string) error {
	_, err := c.Run("config", "--local", "--unset-all", key)
	if ExitCode(err) == 5 {
		return nil
	}
	return err
}

// Repo describes the repository c.Dir is in.
type Repo struct {
	CommonDir string // absolute; shared by all worktrees
	GitDir    string // absolute; this worktree's git dir
	TopLevel  string // absolute worktree root ("" if bare)
}

// RuleDir is the directory a rule must cover to apply to this repo: the parent
// of the common git dir, so linked worktrees (whose git dir lives inside the
// main repo's .git) are covered too (SPEC §5).
func (r Repo) RuleDir() string { return filepath.Dir(r.CommonDir) }

// ErrNotRepo is returned when c.Dir isn't inside a git repository.
var ErrNotRepo = errors.New("not inside a git repository")

// Repo inspects the repository at c.Dir.
func (c Client) Repo() (Repo, error) {
	out, err := c.Run("rev-parse", "--path-format=absolute", "--git-common-dir", "--absolute-git-dir", "--is-bare-repository")
	if ExitCode(err) == 128 {
		return Repo{}, ErrNotRepo
	}
	if err != nil {
		return Repo{}, err
	}
	lines := strings.Split(out, "\n")
	if len(lines) != 3 {
		return Repo{}, fmt.Errorf("unexpected rev-parse output %q", out)
	}
	r := Repo{CommonDir: lines[0], GitDir: lines[1]}
	if lines[2] != "true" {
		top, err := c.Run("rev-parse", "--show-toplevel")
		if err != nil {
			return Repo{}, err
		}
		r.TopLevel = top
	}
	return r, nil
}

var versionRE = regexp.MustCompile(`git version (\d+)\.(\d+)`)

// Version returns git's major and minor version.
func (c Client) Version() (major, minor int, err error) {
	out, err := c.Run("version")
	if err != nil {
		return 0, 0, err
	}
	m := versionRE.FindStringSubmatch(out)
	if m == nil {
		return 0, 0, fmt.Errorf("can't parse %q", out)
	}
	major, _ = strconv.Atoi(m[1])
	minor, _ = strconv.Atoi(m[2])
	return major, minor, nil
}

// RequireVersion errors unless git is at least major.minor.
func (c Client) RequireVersion(major, minor int) error {
	ma, mi, err := c.Version()
	if err != nil {
		return err
	}
	if ma < major || (ma == major && mi < minor) {
		return fmt.Errorf("gids needs git %d.%d or newer (you have %d.%d)", major, minor, ma, mi)
	}
	return nil
}
