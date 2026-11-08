package render

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"unicode"

	"gids/internal/config"
	"gids/internal/testenv"
)

func TestMain(m *testing.M) { testenv.Main(m) }

// gitGet reads key from a config file with real git.
func gitGet(t *testing.T, file, key string) string {
	t.Helper()
	out, err := exec.Command("git", "config", "--file", file, "--get", key).Output()
	if err != nil {
		t.Fatalf("git config --get %s: %v", key, err)
	}
	return strings.TrimSuffix(string(out), "\n")
}

func TestQuoteRoundTripsThroughGit(t *testing.T) {
	file := filepath.Join(t.TempDir(), "c")
	for _, v := range []string{
		"plain", "", " leading and trailing ", `back\slash`, `quote"d`, "tab\there", "# not a comment", "; nor this",
		"ssh -i '/a b/it''s' -o IdentitiesOnly=yes", "Nguyễn Khôi", `\"\\`, "=equals=", "[brackets]",
	} {
		os.WriteFile(file, []byte("[x]\n\tv = "+Quote(v)+"\n"), 0o644)
		if got := gitGet(t, file, "x.v"); got != v {
			t.Errorf("Quote(%q) read back as %q", v, got)
		}
	}
}

func FuzzQuoteRoundTrip(f *testing.F) {
	f.Add("a\\\"b\tc")
	file := filepath.Join(f.TempDir(), "c")
	f.Fuzz(func(t *testing.T, v string) {
		if strings.ContainsAny(v, "\n\r\x00") || !isValidUTF8NoControl(v) {
			t.Skip() // rejected by validation before rendering
		}
		os.WriteFile(file, []byte("[x]\n\tv = "+Quote(v)+"\n"), 0o644)
		if got := gitGet(t, file, "x.v"); got != v {
			t.Fatalf("Quote(%q) read back as %q", v, got)
		}
	})
}

func isValidUTF8NoControl(s string) bool {
	for _, r := range s {
		if r == unicode.ReplacementChar || (unicode.IsControl(r) && r != '\t') {
			return false
		}
	}
	return true
}

func TestRenderProfileReadsBackWithGit(t *testing.T) {
	p := config.Profile{Name: "Work", GitName: `Brian "B" Nguyen`, GitEmail: "b@co.com", Username: "bqn",
		SSHKey: "~/.ssh/it's mine", SigningKey: "~/.ssh/id.pub"}
	file := filepath.Join(t.TempDir(), "p")
	os.WriteFile(file, []byte(renderProfile(p, "/home/b")), 0o644)
	want := map[string]string{
		"user.name":           p.GitName,
		"user.email":          p.GitEmail,
		"user.signingkey":     "/home/b/.ssh/id.pub",
		"gpg.format":          "ssh",
		"core.sshcommand":     `ssh -i '/home/b/.ssh/it'\''s mine' -o IdentitiesOnly=yes`,
		"credential.username": "bqn",
		"gids.profile":        "Work",
	}
	for k, v := range want {
		if got := gitGet(t, file, k); got != v {
			t.Errorf("%s = %q, want %q", k, got, v)
		}
	}
	// The sshCommand must survive sh -c: the quoted path is one argument.
	out, err := exec.Command("sh", "-c", strings.Replace(want["core.sshcommand"], "ssh ", "printf '%s\\n' ", 1)).Output()
	if err != nil || strings.Split(string(out), "\n")[1] != "/home/b/.ssh/it's mine" {
		t.Errorf("sshCommand splits wrongly under sh: %q %v", out, err)
	}
}

func TestRenderProfileOmitsUnset(t *testing.T) {
	out := renderProfile(config.Profile{Name: "A", GitName: "A", GitEmail: "a@b", SigningKey: "ABCDEF12"}, "/h")
	for _, s := range []string{"sshCommand", "credential", "[gpg]"} {
		if strings.Contains(out, s) {
			t.Errorf("unexpected %q in:\n%s", s, out)
		}
	}
	if !strings.Contains(out, `signingKey = "ABCDEF12"`) {
		t.Errorf("GPG key id missing:\n%s", out)
	}
}

func TestRenderRulesOrderAndPaths(t *testing.T) {
	cfg := &config.Config{
		Profiles: []config.Profile{{Name: "A", GitName: "A", GitEmail: "a@b"}, {Name: "B", GitName: "B", GitEmail: "b@c"}},
		Rules:    map[string]string{"~/work/oss/": "B", "~/work/": "A"},
	}
	out := renderRules(cfg, "/home/b")
	i, j := strings.Index(out, `gitdir:~/work/"`), strings.Index(out, `gitdir:~/work/oss/"`)
	if i < 0 || j < 0 || i > j {
		t.Fatalf("most specific rule must come last:\n%s", out)
	}
	if !strings.Contains(out, `path = "profiles/B.gitconfig"`) {
		t.Fatalf("include path wrong:\n%s", out)
	}
}

func TestHooksAreValidShAndQuoteThePaths(t *testing.T) {
	for _, prev := range []string{"", "/opt/my hooks/it's"} {
		for _, h := range Hooks {
			script := renderHook(h, "/opt/gids dir/gi'ds", prev)
			f := filepath.Join(t.TempDir(), h)
			os.WriteFile(f, []byte(script), 0o755)
			if out, err := exec.Command("sh", "-n", f).CombinedOutput(); err != nil {
				t.Fatalf("%s (prev=%q) has a syntax error: %v\n%s\n%s", h, prev, err, out, script)
			}
		}
	}
}

func TestRenderIsDeterministic(t *testing.T) {
	cfg := &config.Config{
		Profiles: []config.Profile{{Name: "A", GitName: "A", GitEmail: "a@b"}, {Name: "B", GitName: "B", GitEmail: "b@c"}},
		Rules:    map[string]string{"~/a/": "A", "~/b/": "B", "~/c/": "A", "/x/": "B"},
	}
	in := Input{Home: "/h", Exe: "/bin/gids"}
	first := Render(cfg, in)
	for range 50 { // map iteration order is random; output must not be
		if got := Render(cfg, in); !equalFiles(got, first) {
			t.Fatal("Render output depends on map order")
		}
	}
}

func equalFiles(a, b []File) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func TestIsSSHSigningKey(t *testing.T) {
	for k, want := range map[string]bool{
		"~/.ssh/id_ed25519.pub": true, "/k/id": true, "key::ssh-ed25519 AAAA": true, "id.pub": true,
		"ABCDEF1234567890": false, "0xDEADBEEF": false, "me@example.com": false,
	} {
		if IsSSHSigningKey(k) != want {
			t.Errorf("IsSSHSigningKey(%q) = %v", k, !want)
		}
	}
}

func TestManaged(t *testing.T) {
	for rel, want := range map[string]bool{
		"gitconfig": true, "profiles/Work.gitconfig": true, "profiles/a.b-c.gitconfig": true,
		"hooks/pre-commit": true, "hooks/commit-msg": true, "hooks/post-commit": true,
		"config.yaml": false, "README.md": false, ".DS_Store": false,
		"profiles/.DS_Store": false, "profiles/notes.txt": false, "profiles/.gitconfig": false,
		"profiles/x/y.gitconfig": false, "hooks/pre-push": false, "hooks/.DS_Store": false,
		"other/gitconfig": false, "profiles": false, "hooks": false,
	} {
		if got := Managed(rel); got != want {
			t.Errorf("Managed(%q) = %v, want %v", rel, got, want)
		}
	}
	// Everything Render produces is managed (or sync would leave it as a stray).
	cfg := &config.Config{Profiles: []config.Profile{{Name: "Work", GitName: "W", GitEmail: "w@x"}}}
	for _, f := range Render(cfg, Input{Home: "/h", Exe: "/bin/gids"}) {
		if !Managed(f.Rel) {
			t.Errorf("rendered file %s isn't Managed", f.Rel)
		}
	}
}
