package config

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"gids/internal/testenv"
)

func TestMain(m *testing.M) { testenv.Main(m) }

func good() Profile {
	return Profile{Name: "Work", GitName: "Brian Nguyen", GitEmail: "brian@company.com"}
}

func TestValidateProfileFields(t *testing.T) {
	cases := []struct {
		name string
		mut  func(*Profile)
		ok   bool
	}{
		{"valid", func(p *Profile) {}, true},
		{"all optional set", func(p *Profile) { p.Username = "bqn"; p.SSHKey = "~/.ssh/id"; p.SigningKey = "ABCDEF" }, true},
		{"name with dot dash underscore", func(p *Profile) { p.Name = "cmu.school-2_x" }, true},
		{"name empty", func(p *Profile) { p.Name = "" }, false},
		{"name with space", func(p *Profile) { p.Name = "My Work" }, false},
		{"name starts with dot", func(p *Profile) { p.Name = ".hidden" }, false},
		{"name ends with dot", func(p *Profile) { p.Name = "work." }, false},
		{"name with slash", func(p *Profile) { p.Name = "a/b" }, false},
		{"name path traversal", func(p *Profile) { p.Name = ".." }, false},
		{"name windows reserved", func(p *Profile) { p.Name = "con" }, false},
		{"name windows reserved COM1", func(p *Profile) { p.Name = "COM1" }, false},
		{"name 64 chars", func(p *Profile) { p.Name = strings.Repeat("a", 64) }, true},
		{"name 65 chars", func(p *Profile) { p.Name = strings.Repeat("a", 65) }, false},
		{"name unicode", func(p *Profile) { p.Name = "trabajo-ñ" }, false},
		{"git name empty", func(p *Profile) { p.GitName = "  " }, false},
		{"git name with angle", func(p *Profile) { p.GitName = "Brian <x>" }, false},
		{"git name with newline", func(p *Profile) { p.GitName = "Brian\n[core]" }, false},
		{"git name leading space", func(p *Profile) { p.GitName = " Brian" }, false},
		{"git name unicode", func(p *Profile) { p.GitName = "Nguyễn Khôi" }, true},
		{"email no at", func(p *Profile) { p.GitEmail = "brian" }, false},
		{"email two at", func(p *Profile) { p.GitEmail = "a@b@c" }, false},
		{"email at start", func(p *Profile) { p.GitEmail = "@b" }, false},
		{"email at end", func(p *Profile) { p.GitEmail = "a@" }, false},
		{"email space", func(p *Profile) { p.GitEmail = "a b@c" }, false},
		{"email plus", func(p *Profile) { p.GitEmail = "a+tag@c.com" }, true},
		{"email control", func(p *Profile) { p.GitEmail = "a@c\x1b" }, false},
		{"username space", func(p *Profile) { p.Username = "a b" }, false},
		{"ssh key relative", func(p *Profile) { p.SSHKey = "id_rsa" }, false},
		{"ssh key absolute with space", func(p *Profile) { p.SSHKey = "/keys/my key" }, true},
		{"ssh key ~user", func(p *Profile) { p.SSHKey = "~bob/.ssh/id" }, false},
		{"ssh key newline", func(p *Profile) { p.SSHKey = "/a\nb" }, false},
		{"signing key padded", func(p *Profile) { p.SigningKey = " ABC" }, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			p := good()
			tc.mut(&p)
			if err := p.Validate(); (err == nil) != tc.ok {
				t.Fatalf("Validate() = %v, want ok=%v", err, tc.ok)
			}
		})
	}
}

func TestNormalizeRule(t *testing.T) {
	home, cwd := "/home/b", "/home/b/projects/foo"
	cases := []struct{ in, want, err string }{
		{"~/work", "~/work/", ""},
		{"~/work/", "~/work/", ""},
		{"~/work//sub/../x", "~/work/x/", ""},
		{"~", "~/", ""},
		{"/home/b/work", "~/work/", ""},
		{"/home/b", "~/", ""},
		{"/home/bob/work", "/home/bob/work/", ""}, // a sibling of home isn't inside it
		{".", "~/projects/foo/", ""},
		{"..", "~/projects/", ""},
		{"sub dir", "~/projects/foo/sub dir/", ""},
		{"/opt/src", "/opt/src/", ""},
		{"/", "/", ""},
		{"~/work/*", "~/work/*/", ""},
		{"  ~/work  ", "~/work/", ""},
		{"", "", "required"},
		{"~bob/work", "", "~user"},
		{`~/a"b`, "", `'"'`},
		{`/a\b`, "", `'\'`},
		{"~/a\nb", "", "control"},
	}
	for _, tc := range cases {
		got, err := NormalizeRule(tc.in, cwd, home)
		if tc.err != "" {
			if err == nil || !strings.Contains(err.Error(), tc.err) {
				t.Errorf("NormalizeRule(%q) err = %v, want containing %q", tc.in, err, tc.err)
			}
			continue
		}
		if err != nil || got != tc.want {
			t.Errorf("NormalizeRule(%q) = %q, %v; want %q", tc.in, got, err, tc.want)
		}
		if err := ValidateRuleKey(got); err != nil {
			t.Errorf("normalized %q fails ValidateRuleKey: %v", got, err)
		}
		again, err := NormalizeRule(got, "/elsewhere", home)
		if err != nil || again != got {
			t.Errorf("not idempotent: %q -> %q (%v)", got, again, err)
		}
	}
}

func TestValidateRuleKeyRejectsUnnormalized(t *testing.T) {
	for _, k := range []string{"~/work", "work/", "~/a/../b/", "~/a//b/", "~/./a/", "relative/x/", "~bob/x/", `~/a"/`} {
		if ValidateRuleKey(k) == nil {
			t.Errorf("ValidateRuleKey(%q) accepted an unnormalized key", k)
		}
	}
	for _, k := range []string{"~/", "/", "~/work/", "/opt/x y/", "C:/Users/b/work/"} {
		if err := ValidateRuleKey(k); err != nil {
			t.Errorf("ValidateRuleKey(%q) = %v", k, err)
		}
	}
}

func TestSortedRulesMostSpecificLast(t *testing.T) {
	rules := map[string]string{
		"~/work/oss/thing/": "C", "~/": "Z", "~/work/": "A", "/": "R", "~/work/oss/": "B", "/opt/": "O",
	}
	got := SortedRules(rules, "/home/b")
	want := []string{"/", "/opt/", "~/", "~/work/", "~/work/oss/", "~/work/oss/thing/"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("SortedRules = %v, want %v", got, want)
	}
	// Order depends on depth, not on how long the home path is.
	if got := SortedRules(map[string]string{"~/a/": "x", "/very/long/path/name/": "y"}, "/h"); got[0] != "~/a/" {
		t.Fatalf("depth ordering wrong with short home: %v", got)
	}
}

func TestLoadAndSave(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")

	cfg, err := Load(path)
	if err != nil || len(cfg.Profiles) != 0 {
		t.Fatalf("missing file: %v %v", cfg, err)
	}
	os.WriteFile(path, nil, 0o600)
	if cfg, err = Load(path); err != nil || len(cfg.Profiles) != 0 {
		t.Fatalf("empty file: %v %v", cfg, err)
	}

	cfg = &Config{Profiles: []Profile{good()}, Rules: map[string]string{"~/work/": "Work"}}
	if err := Save(path, cfg); err != nil {
		t.Fatal(err)
	}
	fi, _ := os.Stat(path)
	if fi.Mode().Perm() != 0o600 {
		t.Errorf("config mode = %v, want 0600", fi.Mode().Perm())
	}
	back, err := Load(path)
	if err != nil || !reflect.DeepEqual(back, cfg) {
		t.Fatalf("round trip: %+v %v", back, err)
	}
	entries, _ := os.ReadDir(dir)
	if len(entries) != 1 {
		t.Errorf("temp files left behind: %v", entries)
	}
}

func TestLoadRejectsBadFilesWithoutTouchingThem(t *testing.T) {
	cases := map[string]string{
		"unknown key":       "profiles: []\nrulez: {}\n",
		"bad yaml":          "profiles: [\n",
		"rule to missing":   "profiles: []\nrules:\n  ~/x/: Ghost\n",
		"dup name (case)":   "profiles:\n- {name: Work, git_name: A, git_email: a@b}\n- {name: work, git_name: B, git_email: b@c}\n",
		"invalid email":     "profiles:\n- {name: Work, git_name: A, git_email: nope}\n",
		"unnormalized rule": "profiles:\n- {name: Work, git_name: A, git_email: a@b}\nrules:\n  ~/x: Work\n",
		"v1 glob rule":      "profiles:\n- {name: Work, git_name: A, git_email: a@b}\nrules:\n  ~/work/*: Work\n",
	}
	for name, content := range cases {
		t.Run(name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "config.yaml")
			os.WriteFile(path, []byte(content), 0o600)
			if _, err := Load(path); err == nil || !strings.Contains(err.Error(), path) {
				t.Fatalf("Load = %v, want error naming the file", err)
			}
			if b, _ := os.ReadFile(path); string(b) != content {
				t.Fatal("Load modified the file")
			}
		})
	}
}

func TestSaveRefusesInvalid(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yaml")
	bad := &Config{Profiles: []Profile{good()}, Rules: map[string]string{"~/x/": "Nobody"}}
	if err := Save(path, bad); err == nil {
		t.Fatal("Save accepted a rule to a missing profile")
	}
	if _, err := os.Stat(path); err == nil {
		t.Fatal("Save wrote an invalid config")
	}
}

func TestProfileMutations(t *testing.T) {
	cfg := &Config{}
	if err := cfg.AddProfile(good()); err != nil {
		t.Fatal(err)
	}
	dup := good()
	dup.Name = "WORK"
	if err := cfg.AddProfile(dup); err == nil {
		t.Fatal("AddProfile allowed a case-insensitive duplicate")
	}
	p2 := Profile{Name: "Personal", GitName: "B", GitEmail: "b@x"}
	cfg.AddProfile(p2)
	cfg.SetRule("~/work/", "Work")
	cfg.SetRule("~/me/", "Personal")

	renamed := good()
	renamed.Name = "Job"
	if err := cfg.ReplaceProfile("Work", renamed); err != nil {
		t.Fatal(err)
	}
	if cfg.Rules["~/work/"] != "Job" || cfg.Profile("Work") != nil {
		t.Fatalf("rename didn't carry rules: %v", cfg.Rules)
	}
	clash := good()
	clash.Name = "personal"
	if err := cfg.ReplaceProfile("Job", clash); err == nil {
		t.Fatal("rename onto another profile's name allowed")
	}
	if err := cfg.ReplaceProfile("Ghost", good()); err == nil {
		t.Fatal("replacing a missing profile allowed")
	}
	cfg.DeleteProfile("Job")
	if _, ok := cfg.Rules["~/work/"]; ok || len(cfg.Profiles) != 1 {
		t.Fatalf("delete left rules/profile: %v %v", cfg.Rules, cfg.Profiles)
	}
	if err := cfg.Validate(); err != nil {
		t.Fatalf("config invalid after mutations: %v", err)
	}
}

func TestResolvePaths(t *testing.T) {
	s := testenv.New(t)
	p, err := ResolvePaths()
	if err != nil {
		t.Fatal(err)
	}
	if p.Dir != filepath.Join(s.Home, ".config", "gids") || p.GlobalGitconfig != filepath.Join(s.Home, ".gitconfig") {
		t.Fatalf("paths: %+v", p)
	}
	t.Setenv("XDG_CONFIG_HOME", "relative/ignored")
	t.Setenv("GIT_CONFIG_GLOBAL", "")
	p, _ = ResolvePaths()
	if p.Dir != filepath.Join(s.Home, ".config", "gids") {
		t.Fatalf("relative XDG_CONFIG_HOME should be ignored: %s", p.Dir)
	}
	// Relative paths would make gids write into the current directory.
	t.Setenv("GIT_CONFIG_GLOBAL", "rel/.gitconfig")
	if _, err := ResolvePaths(); err == nil {
		t.Fatal("relative GIT_CONFIG_GLOBAL accepted")
	}
	t.Setenv("GIT_CONFIG_GLOBAL", "")
	t.Setenv("HOME", "relative-home")
	if _, err := ResolvePaths(); err == nil {
		t.Fatal("relative HOME accepted")
	}
}

func FuzzNormalizeRule(f *testing.F) {
	for _, s := range []string{"~/work", ".", "../x", "/a/b/../c", "~", "a//b", "~/x/*", ""} {
		f.Add(s)
	}
	f.Fuzz(func(t *testing.T, in string) {
		got, err := NormalizeRule(in, "/home/b/cwd", "/home/b")
		if err != nil {
			return
		}
		if err := ValidateRuleKey(got); err != nil {
			t.Fatalf("NormalizeRule(%q) = %q, which is invalid: %v", in, got, err)
		}
		if again, err := NormalizeRule(got, "/other", "/home/b"); err != nil || again != got {
			t.Fatalf("not idempotent: %q -> %q -> %q (%v)", in, got, again, err)
		}
	})
}
