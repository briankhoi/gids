package git

import (
	"fmt"
	"strings"

	"gids/internal/config"
)

// Apply writes a profile's Git identity fields into the local .git/config.
//
// Required fields (user.name, user.email) are always set. Optional fields
// (core.sshCommand, credential.username, user.signingKey) are set when the
// profile field is non-empty and unset otherwise, so switching profiles never
// leaves stale values behind.
func Apply(c *Client, p config.Profile) error {
	if err := c.ConfigSet("user.name", p.GitName); err != nil {
		return fmt.Errorf("setting user.name: %w", err)
	}
	if err := c.ConfigSet("user.email", p.GitEmail); err != nil {
		return fmt.Errorf("setting user.email: %w", err)
	}

	if err := setOrUnset(c, "core.sshCommand", sshCommand(p.SSHKey)); err != nil {
		return err
	}
	if err := setOrUnset(c, "credential.username", p.Username); err != nil {
		return err
	}
	if err := setOrUnset(c, "user.signingKey", p.SigningKey); err != nil {
		return err
	}

	return nil
}

// setOrUnset sets key to value when value is non-empty, otherwise unsets it.
func setOrUnset(c *Client, key, value string) error {
	if value != "" {
		if err := c.ConfigSet(key, value); err != nil {
			return fmt.Errorf("setting %s: %w", key, err)
		}
		return nil
	}
	if err := c.ConfigUnset(key); err != nil {
		return fmt.Errorf("unsetting %s: %w", key, err)
	}
	return nil
}

// sshCommand returns the core.sshCommand value for a given key path, or ""
// when the path is empty (meaning the key should be unset).
//
// core.sshCommand is passed to sh -c by git, so the key path must be
// single-quoted to prevent shell metacharacter interpretation (spaces, $, ;,
// |, backticks, etc.). Any embedded single quotes are escaped with '\''.
func sshCommand(keyPath string) string {
	if keyPath == "" {
		return ""
	}
	escaped := strings.ReplaceAll(keyPath, "'", `'\''`)
	return "ssh -i '" + escaped + "'"
}
