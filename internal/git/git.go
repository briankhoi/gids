package git

import (
	"bytes"
	"errors"
	"fmt"
	"os/exec"
	"strings"
)

// Client wraps local git-config operations for a specific directory.
type Client struct {
	dir string
}

// New returns a Client rooted at dir.
// Pass "" or "." to use the current working directory.
func New(dir string) *Client {
	return &Client{dir: dir}
}

// IsRepo reports whether dir is inside a git repository.
func (c *Client) IsRepo() (bool, error) {
	if err := c.runDiscard("rev-parse", "--git-dir"); err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) && exitErr.ExitCode() == 128 {
			return false, nil
		}
		return false, err
	}
	return true, nil
}

// ConfigSet sets a local git config key to value.
func (c *Client) ConfigSet(key, value string) error {
	out, err := c.run("config", "--local", key, value)
	if err != nil {
		return fmt.Errorf("git config set %s: %w\n%s", key, err, strings.TrimSpace(out))
	}
	return nil
}

// ConfigGet reads a local git config value.
// Returns "", nil if the key is not set.
func (c *Client) ConfigGet(key string) (string, error) {
	out, err := c.run("config", "--local", "--get", key)
	if err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) && exitErr.ExitCode() == 1 {
			// exit 1: key does not exist
			return "", nil
		}
		return "", fmt.Errorf("git config get %s: %w", key, err)
	}
	return strings.TrimRight(out, "\n"), nil
}

// ConfigGetEffective reads the effective (local → global → system) git config
// value for key. Returns "", nil if the key is not set at any level.
func (c *Client) ConfigGetEffective(key string) (string, error) {
	out, err := c.run("config", "--get", key)
	if err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) && exitErr.ExitCode() == 1 {
			// exit 1: key does not exist at any level
			return "", nil
		}
		return "", fmt.Errorf("git config get %s: %w", key, err)
	}
	return strings.TrimRight(out, "\n"), nil
}

// ConfigSetGlobal sets a global git config key to value.
func ConfigSetGlobal(key, value string) error {
	cmd := exec.Command("git", "config", "--global", key, value)
	var buf bytes.Buffer
	cmd.Stdout = &buf
	cmd.Stderr = &buf
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("git config global set %s: %w\n%s", key, err, strings.TrimSpace(buf.String()))
	}
	return nil
}

// ConfigGetGlobal reads a global git config value.
// Returns "", nil if the key is not set.
func ConfigGetGlobal(key string) (string, error) {
	cmd := exec.Command("git", "config", "--global", "--get", key)
	var buf bytes.Buffer
	cmd.Stdout = &buf
	cmd.Stderr = &buf
	if err := cmd.Run(); err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) && exitErr.ExitCode() == 1 {
			return "", nil // key does not exist at global level
		}
		return "", fmt.Errorf("git config global get %s: %w", key, err)
	}
	return strings.TrimRight(buf.String(), "\n"), nil
}

// ConfigUnsetGlobal removes a global git config key.
// Returns nil if the key is not set.
func ConfigUnsetGlobal(key string) error {
	cmd := exec.Command("git", "config", "--global", "--unset", key)
	var buf bytes.Buffer
	cmd.Stdout = &buf
	cmd.Stderr = &buf
	if err := cmd.Run(); err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) && exitErr.ExitCode() == 5 {
			return nil // key does not exist
		}
		return fmt.Errorf("git config global unset %s: %w\n%s", key, err, strings.TrimSpace(buf.String()))
	}
	return nil
}

// ConfigUnset removes a local git config key.
// Returns nil if the key is not set.
func (c *Client) ConfigUnset(key string) error {
	out, err := c.run("config", "--local", "--unset", key)
	if err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) && exitErr.ExitCode() == 5 {
			// exit 5: key does not exist
			return nil
		}
		return fmt.Errorf("git config unset %s: %w\n%s", key, err, strings.TrimSpace(out))
	}
	return nil
}

// run executes git with the given args in c.dir and returns combined output.
func (c *Client) run(args ...string) (string, error) {
	cmd := exec.Command("git", args...)
	cmd.Dir = c.dir
	var buf bytes.Buffer
	cmd.Stdout = &buf
	cmd.Stderr = &buf
	err := cmd.Run() // buf is populated only after Run() completes
	return buf.String(), err
}

// runDiscard executes git with the given args in c.dir, discarding output.
func (c *Client) runDiscard(args ...string) error {
	cmd := exec.Command("git", args...)
	cmd.Dir = c.dir
	return cmd.Run()
}
