package cmd

import (
	"testing"

	"gids/internal/config"
)

// TestEditedProfile_AppliesAllFields verifies that editedProfile returns a new
// Profile with all provided field values applied.
func TestEditedProfile_AppliesAllFields(t *testing.T) {
	base := config.Profile{
		Name:       "Work",
		GitName:    "Alice Old",
		GitEmail:   "old@example.com",
		Username:   "old-user",
		SSHKey:     "/old/key",
		SigningKey:  "OLD_KEY",
	}

	got := editedProfile(base, "Alice New", "new@example.com", "new-user", "/new/key", "NEW_KEY")

	if got.Name != "Work" {
		t.Errorf("Name = %q, want %q", got.Name, "Work")
	}
	if got.GitName != "Alice New" {
		t.Errorf("GitName = %q, want %q", got.GitName, "Alice New")
	}
	if got.GitEmail != "new@example.com" {
		t.Errorf("GitEmail = %q, want %q", got.GitEmail, "new@example.com")
	}
	if got.Username != "new-user" {
		t.Errorf("Username = %q, want %q", got.Username, "new-user")
	}
	if got.SSHKey != "/new/key" {
		t.Errorf("SSHKey = %q, want %q", got.SSHKey, "/new/key")
	}
	if got.SigningKey != "NEW_KEY" {
		t.Errorf("SigningKey = %q, want %q", got.SigningKey, "NEW_KEY")
	}
}

// TestEditedProfile_DoesNotMutateBase verifies that editedProfile never modifies
// the original profile — the base value must be unchanged after the call.
func TestEditedProfile_DoesNotMutateBase(t *testing.T) {
	base := config.Profile{
		Name:     "Work",
		GitName:  "Alice",
		GitEmail: "alice@example.com",
	}

	_ = editedProfile(base, "Bob", "bob@example.com", "", "", "")

	if base.GitName != "Alice" {
		t.Errorf("base.GitName was mutated: got %q, want %q", base.GitName, "Alice")
	}
	if base.GitEmail != "alice@example.com" {
		t.Errorf("base.GitEmail was mutated: got %q, want %q", base.GitEmail, "alice@example.com")
	}
}
