package cmd

import (
	"os"
	"path/filepath"
	"testing"
)

// --- homeDir ---

func TestHomeDir_ReturnsNonEmpty(t *testing.T) {
	got, err := homeDir()
	if err != nil {
		t.Fatalf("homeDir: %v", err)
	}
	if got == "" {
		t.Error("homeDir returned empty string")
	}
	if !filepath.IsAbs(got) {
		t.Errorf("homeDir returned non-absolute path: %q", got)
	}
}

// --- tildify ---

func TestTildify_ReplacesHomePrefix(t *testing.T) {
	home, err := os.UserHomeDir()
	if err != nil {
		t.Skip("cannot determine home directory")
	}

	path := filepath.Join(home, ".ssh", "config")
	got := tildify(path)
	want := "~/.ssh/config"
	if got != want {
		t.Errorf("tildify(%q) = %q, want %q", path, got, want)
	}
}

func TestTildify_ExactHome(t *testing.T) {
	home, err := os.UserHomeDir()
	if err != nil {
		t.Skip("cannot determine home directory")
	}

	got := tildify(home)
	if got != "~" {
		t.Errorf("tildify(home) = %q, want %q", got, "~")
	}
}

func TestTildify_NonHomePath_Unchanged(t *testing.T) {
	path := "/etc/ssh/ssh_config"
	got := tildify(path)
	if got != path {
		t.Errorf("tildify(%q) = %q, want unchanged %q", path, got, path)
	}
}

func TestTildify_EmptyPath_Unchanged(t *testing.T) {
	got := tildify("")
	if got != "" {
		t.Errorf("tildify(\"\") = %q, want empty string", got)
	}
}
