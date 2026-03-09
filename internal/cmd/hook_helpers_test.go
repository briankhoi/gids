package cmd

import (
	"strings"
	"testing"
)

// --- shellScript ---

func TestShellScript_AllSupportedShells(t *testing.T) {
	for _, shell := range []string{"zsh", "bash", "fish", "powershell"} {
		t.Run(shell, func(t *testing.T) {
			script, err := shellScript(shell)
			if err != nil {
				t.Fatalf("shellScript(%q): %v", shell, err)
			}
			if !strings.Contains(script, "gids check") {
				t.Errorf("shellScript(%q) does not contain 'gids check'", shell)
			}
			if !strings.Contains(script, hookBeginMarker) {
				t.Errorf("shellScript(%q) missing begin marker", shell)
			}
			if !strings.Contains(script, hookEndMarker) {
				t.Errorf("shellScript(%q) missing end marker", shell)
			}
		})
	}
}

func TestShellScript_UnknownShell(t *testing.T) {
	_, err := shellScript("tcsh")
	if err == nil {
		t.Error("expected error for unknown shell")
	}
	if !strings.Contains(err.Error(), "unsupported") {
		t.Errorf("expected 'unsupported' in error, got: %v", err)
	}
}

// --- hookInstalled ---

func TestHookInstalled_True(t *testing.T) {
	content := "some config\n" + hookBeginMarker + "\nscript\n" + hookEndMarker + "\n"
	if !hookInstalled(content) {
		t.Error("expected hookInstalled=true when markers present")
	}
}

func TestHookInstalled_False(t *testing.T) {
	if hookInstalled("some config without hook") {
		t.Error("expected hookInstalled=false when markers absent")
	}
}

func TestHookInstalled_EmptyString(t *testing.T) {
	if hookInstalled("") {
		t.Error("expected hookInstalled=false for empty string")
	}
}

// --- addHook ---

func TestAddHook_AppendsToEmpty(t *testing.T) {
	script := hookBeginMarker + "\nscript content\n" + hookEndMarker
	result := addHook("", script)
	if !strings.Contains(result, "script content") {
		t.Errorf("addHook result missing script content: %q", result)
	}
	if !strings.Contains(result, hookBeginMarker) {
		t.Errorf("addHook result missing begin marker")
	}
}

func TestAddHook_AppendsToExistingContent(t *testing.T) {
	existing := "export PATH=/usr/local/bin:$PATH\n"
	script := hookBeginMarker + "\nscript content\n" + hookEndMarker
	result := addHook(existing, script)
	if !strings.HasPrefix(result, "export PATH") {
		t.Errorf("addHook should preserve existing content, got: %q", result)
	}
	if !strings.Contains(result, "script content") {
		t.Errorf("addHook result missing script content")
	}
}

func TestAddHook_ReplacesExistingHook(t *testing.T) {
	old := hookBeginMarker + "\nold script\n" + hookEndMarker
	new := hookBeginMarker + "\nnew script\n" + hookEndMarker
	content := "config preamble\n" + old + "\nmore config\n"

	result := addHook(content, new)
	if strings.Contains(result, "old script") {
		t.Errorf("addHook should replace old hook, still contains 'old script': %q", result)
	}
	if !strings.Contains(result, "new script") {
		t.Errorf("addHook result missing 'new script': %q", result)
	}
	if !strings.Contains(result, "more config") {
		t.Errorf("addHook should preserve content after old hook: %q", result)
	}
}

func TestAddHook_OnlyOneBeginMarker(t *testing.T) {
	existing := "preamble\n"
	script := hookBeginMarker + "\nscript\n" + hookEndMarker
	result := addHook(existing, script)
	if count := strings.Count(result, hookBeginMarker); count != 1 {
		t.Errorf("expected 1 begin marker, got %d: %q", count, result)
	}
}

// --- removeHook ---

func TestRemoveHook_RemovesBlock(t *testing.T) {
	hook := hookBeginMarker + "\nscript\n" + hookEndMarker
	content := "preamble\n" + hook + "\npostamble\n"

	result := removeHook(content)
	if strings.Contains(result, "script") {
		t.Errorf("removeHook should remove script, got: %q", result)
	}
	if strings.Contains(result, hookBeginMarker) {
		t.Errorf("removeHook should remove begin marker, got: %q", result)
	}
	if !strings.Contains(result, "postamble") {
		t.Errorf("removeHook should preserve postamble, got: %q", result)
	}
}

func TestRemoveHook_NoHook_Unchanged(t *testing.T) {
	content := "just regular config\n"
	if got := removeHook(content); got != content {
		t.Errorf("removeHook returned %q, want unchanged %q", got, content)
	}
}

func TestRemoveHook_EmptyString(t *testing.T) {
	if got := removeHook(""); got != "" {
		t.Errorf("removeHook(\"\") = %q, want empty", got)
	}
}

func TestRemoveHook_MissingEndMarker_Unchanged(t *testing.T) {
	// A truncated config (begin marker but no end marker) must be returned
	// unchanged rather than silently deleting content.
	content := "preamble\n" + hookBeginMarker + "\norphaned block"
	if got := removeHook(content); got != content {
		t.Errorf("removeHook with missing end marker = %q, want unchanged %q", got, content)
	}
}

// --- defaultShellConfigPath ---

func TestDefaultShellConfigPath_AllSupportedShells(t *testing.T) {
	for _, shell := range []string{"zsh", "bash", "fish", "powershell"} {
		t.Run(shell, func(t *testing.T) {
			path, err := defaultShellConfigPath(shell)
			if err != nil {
				t.Fatalf("defaultShellConfigPath(%q): %v", shell, err)
			}
			if path == "" {
				t.Errorf("defaultShellConfigPath(%q) returned empty string", shell)
			}
		})
	}
}

func TestDefaultShellConfigPath_UnknownShell(t *testing.T) {
	_, err := defaultShellConfigPath("tcsh")
	if err == nil {
		t.Error("expected error for unknown shell")
	}
}
