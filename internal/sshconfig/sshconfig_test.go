package sshconfig_test

import (
	"os"
	"path/filepath"
	"testing"

	"gids/internal/sshconfig"
	"gids/internal/testutil"
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
	content := `Host ` + testutil.SSHHostWork + `
  HostName work.example.com
  User ` + testutil.SSHUser + `
  IdentityFile ` + testutil.SSHKey + `

Host ` + testutil.SSHHostPersonal + `
  HostName personal.example.com
  User ` + testutil.SSHUser + `
  IdentityFile ` + testutil.SSHKey2 + `
`
	path := writeTempConfig(t, content)

	hosts, err := sshconfig.ParseFile(path)
	if err != nil {
		t.Fatalf("ParseFile: %v", err)
	}
	if len(hosts) != 2 {
		t.Fatalf("expected 2 hosts, got %d", len(hosts))
	}

	if hosts[0].Pattern != testutil.SSHHostWork {
		t.Errorf("hosts[0].Pattern = %q, want %q", hosts[0].Pattern, testutil.SSHHostWork)
	}
	if hosts[0].User != testutil.SSHUser {
		t.Errorf("hosts[0].User = %q, want %q", hosts[0].User, testutil.SSHUser)
	}
	if hosts[0].IdentityFile != testutil.SSHKey {
		t.Errorf("hosts[0].IdentityFile = %q, want %q", hosts[0].IdentityFile, testutil.SSHKey)
	}

	if hosts[1].Pattern != testutil.SSHHostPersonal {
		t.Errorf("hosts[1].Pattern = %q, want %q", hosts[1].Pattern, testutil.SSHHostPersonal)
	}
	if hosts[1].IdentityFile != testutil.SSHKey2 {
		t.Errorf("hosts[1].IdentityFile = %q, want %q", hosts[1].IdentityFile, testutil.SSHKey2)
	}
}

func TestParseFile_SkipsWildcard(t *testing.T) {
	content := `Host *
  ServerAliveInterval 60

Host ` + testutil.SSHHostWork + `
  IdentityFile ` + testutil.SSHKey + `
`
	path := writeTempConfig(t, content)

	hosts, err := sshconfig.ParseFile(path)
	if err != nil {
		t.Fatalf("ParseFile: %v", err)
	}
	if len(hosts) != 1 {
		t.Fatalf("expected 1 host (wildcard skipped), got %d", len(hosts))
	}
	if hosts[0].Pattern != testutil.SSHHostWork {
		t.Errorf("hosts[0].Pattern = %q, want %q", hosts[0].Pattern, testutil.SSHHostWork)
	}
}

// TestParseFile_SkipsQuestionMarkWildcard verifies that ? patterns are also
// treated as wildcards and skipped, not just * patterns.
func TestParseFile_SkipsQuestionMarkWildcard(t *testing.T) {
	content := `Host host?.example.com
  ServerAliveInterval 60

Host ` + testutil.SSHHostWork + `
  IdentityFile ` + testutil.SSHKey + `
`
	path := writeTempConfig(t, content)

	hosts, err := sshconfig.ParseFile(path)
	if err != nil {
		t.Fatalf("ParseFile: %v", err)
	}
	if len(hosts) != 1 {
		t.Fatalf("expected 1 host (? wildcard skipped), got %d: %+v", len(hosts), hosts)
	}
	if hosts[0].Pattern != testutil.SSHHostWork {
		t.Errorf("hosts[0].Pattern = %q, want %q", hosts[0].Pattern, testutil.SSHHostWork)
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
	content := `Host ` + testutil.SSHHostWork + `
  IdentityFile ` + testutil.SSHKey + `
  IdentityFile ` + testutil.SSHKey2 + `
`
	path := writeTempConfig(t, content)

	hosts, err := sshconfig.ParseFile(path)
	if err != nil {
		t.Fatalf("ParseFile: %v", err)
	}
	if len(hosts) != 1 {
		t.Fatalf("expected 1 host, got %d", len(hosts))
	}
	if hosts[0].IdentityFile != testutil.SSHKey {
		t.Errorf("IdentityFile = %q, want first entry %q", hosts[0].IdentityFile, testutil.SSHKey)
	}
}

func TestParseFile_NoIdentityFile(t *testing.T) {
	content := `Host ` + testutil.SSHHostWork + `
  User ` + testutil.SSHUser + `
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
