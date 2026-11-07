package sshconfig_test

import (
	"os"
	"path/filepath"
	"testing"

	"gids/internal/sshconfig"
)

func writeTempConfig(t *testing.T, content string) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "config")
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("writeTempConfig: %v", err)
	}
	return path
}

func TestParseFile_MultipleHosts(t *testing.T) {
	content := `Host ` + hostWork + `
  HostName work.example.com
  User ` + sshUser + `
  IdentityFile ` + key1 + `

Host ` + hostPersonal + `
  HostName personal.example.com
  User ` + sshUser + `
  IdentityFile ` + key2 + `
`
	path := writeTempConfig(t, content)

	hosts, err := sshconfig.ParseFile(path)
	if err != nil {
		t.Fatalf("ParseFile: %v", err)
	}
	if len(hosts) != 2 {
		t.Fatalf("expected 2 hosts, got %d", len(hosts))
	}

	if hosts[0].Pattern != hostWork {
		t.Errorf("hosts[0].Pattern = %q, want %q", hosts[0].Pattern, hostWork)
	}
	if hosts[0].User != sshUser {
		t.Errorf("hosts[0].User = %q, want %q", hosts[0].User, sshUser)
	}
	if hosts[0].IdentityFile != key1 {
		t.Errorf("hosts[0].IdentityFile = %q, want %q", hosts[0].IdentityFile, key1)
	}

	if hosts[1].Pattern != hostPersonal {
		t.Errorf("hosts[1].Pattern = %q, want %q", hosts[1].Pattern, hostPersonal)
	}
	if hosts[1].IdentityFile != key2 {
		t.Errorf("hosts[1].IdentityFile = %q, want %q", hosts[1].IdentityFile, key2)
	}
}

func TestParseFile_SkipsWildcard(t *testing.T) {
	content := `Host *
  ServerAliveInterval 60

Host ` + hostWork + `
  IdentityFile ` + key1 + `
`
	path := writeTempConfig(t, content)

	hosts, err := sshconfig.ParseFile(path)
	if err != nil {
		t.Fatalf("ParseFile: %v", err)
	}
	if len(hosts) != 1 {
		t.Fatalf("expected 1 host (wildcard skipped), got %d", len(hosts))
	}
	if hosts[0].Pattern != hostWork {
		t.Errorf("hosts[0].Pattern = %q, want %q", hosts[0].Pattern, hostWork)
	}
}

// TestParseFile_SkipsQuestionMarkWildcard verifies that ? patterns are also
// treated as wildcards and skipped, not just * patterns.
func TestParseFile_SkipsQuestionMarkWildcard(t *testing.T) {
	content := `Host host?.example.com
  ServerAliveInterval 60

Host ` + hostWork + `
  IdentityFile ` + key1 + `
`
	path := writeTempConfig(t, content)

	hosts, err := sshconfig.ParseFile(path)
	if err != nil {
		t.Fatalf("ParseFile: %v", err)
	}
	if len(hosts) != 1 {
		t.Fatalf("expected 1 host (? wildcard skipped), got %d: %+v", len(hosts), hosts)
	}
	if hosts[0].Pattern != hostWork {
		t.Errorf("hosts[0].Pattern = %q, want %q", hosts[0].Pattern, hostWork)
	}
}

// TestDefaultConfigPaths_ReturnsAbsolutePaths verifies that DefaultConfigPaths
// returns only absolute paths and no error under normal conditions.
func TestDefaultConfigPaths_ReturnsAbsolutePaths(t *testing.T) {
	paths, err := sshconfig.DefaultConfigPaths()
	if err != nil {
		t.Fatalf("DefaultConfigPaths: %v", err)
	}
	if len(paths) == 0 {
		t.Fatal("expected at least one candidate path")
	}
	for _, p := range paths {
		if !filepath.IsAbs(p) {
			t.Errorf("path %q is not absolute", p)
		}
	}
}

func TestParseFile_FirstIdentityFileWins(t *testing.T) {
	content := `Host ` + hostWork + `
  IdentityFile ` + key1 + `
  IdentityFile ` + key2 + `
`
	path := writeTempConfig(t, content)

	hosts, err := sshconfig.ParseFile(path)
	if err != nil {
		t.Fatalf("ParseFile: %v", err)
	}
	if len(hosts) != 1 {
		t.Fatalf("expected 1 host, got %d", len(hosts))
	}
	if hosts[0].IdentityFile != key1 {
		t.Errorf("IdentityFile = %q, want first entry %q", hosts[0].IdentityFile, key1)
	}
}

func TestParseFile_NoIdentityFile(t *testing.T) {
	content := `Host ` + hostWork + `
  User ` + sshUser + `
`
	path := writeTempConfig(t, content)

	hosts, err := sshconfig.ParseFile(path)
	if err != nil {
		t.Fatalf("ParseFile: %v", err)
	}
	if len(hosts) != 1 {
		t.Fatalf("expected 1 host, got %d", len(hosts))
	}
	if hosts[0].IdentityFile != "" {
		t.Errorf("expected empty IdentityFile, got %q", hosts[0].IdentityFile)
	}
}

func TestParseFile_UnreadableFile(t *testing.T) {
	_, err := sshconfig.ParseFile("/nonexistent/path/config")
	if err == nil {
		t.Fatal("expected error for missing file, got nil")
	}
}

func TestParseFile_EmptyConfig(t *testing.T) {
	path := writeTempConfig(t, "")
	hosts, err := sshconfig.ParseFile(path)
	if err != nil {
		t.Fatalf("ParseFile: %v", err)
	}
	if len(hosts) != 0 {
		t.Errorf("expected 0 hosts, got %d", len(hosts))
	}
}

const (
	hostWork     = "work-server"
	hostPersonal = "personal-vps"
	sshUser      = "git"
	key1         = "~/.ssh/id_example"
	key2         = "~/.ssh/id_personal"
)

func TestFilter(t *testing.T) {
	hosts := []sshconfig.Host{{Pattern: "github-work"}, {Pattern: "GitHub-Personal"}, {Pattern: "vps"}}
	names := func(hs []sshconfig.Host) (out []string) {
		for _, h := range hs {
			out = append(out, h.Pattern)
		}
		return out
	}
	if got := names(sshconfig.Filter(hosts, "")); len(got) != 3 {
		t.Errorf("empty filter must keep all: %v", got)
	}
	if got := names(sshconfig.Filter(hosts, "GITHUB")); len(got) != 2 || got[0] != "github-work" || got[1] != "GitHub-Personal" {
		t.Errorf("case-insensitive, order kept: %v", got)
	}
	if got := sshconfig.Filter(hosts, "nope"); len(got) != 0 {
		t.Errorf("no match: %v", got)
	}
}
