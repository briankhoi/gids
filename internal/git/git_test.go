package git

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"gids/internal/testenv"
)

func TestMain(m *testing.M) { testenv.Main(m) }

func TestRepoInfoIncludingWorktree(t *testing.T) {
	s := testenv.New(t)
	dir := s.Repo("proj", true)
	r, err := Client{Dir: filepath.Join(dir)}.Repo()
	if err != nil {
		t.Fatal(err)
	}
	if r.RuleDir() != dir || r.TopLevel != dir {
		t.Fatalf("repo %+v", r)
	}
	wt := s.Path("elsewhere", "wt")
	s.Git(dir, "worktree", "add", "-q", wt)
	w, err := Client{Dir: wt}.Repo()
	if err != nil {
		t.Fatal(err)
	}
	if w.RuleDir() != dir {
		t.Fatalf("worktree RuleDir = %s, want main repo %s", w.RuleDir(), dir)
	}
	if _, err := (Client{Dir: s.Home}).Repo(); !errors.Is(err, ErrNotRepo) {
		t.Fatalf("home isn't a repo: %v", err)
	}
}

func TestConfigHelpers(t *testing.T) {
	s := testenv.New(t)
	dir := s.Repo("p", false)
	g := Client{Dir: dir}
	if _, ok, err := g.ConfigGet("user.email"); ok || err != nil {
		t.Fatalf("unset key: ok=%v err=%v", ok, err)
	}
	s.Git(dir, "config", "user.email", "local@x")
	src, ok, err := g.ConfigSource("user.email")
	if err != nil || !ok || src.Scope != "local" || src.Value != "local@x" {
		t.Fatalf("source %+v ok=%v err=%v", src, ok, err)
	}
	if err := g.ConfigUnsetLocal("user.email"); err != nil {
		t.Fatal(err)
	}
	if err := g.ConfigUnsetLocal("user.email"); err != nil { // already unset is fine
		t.Fatal(err)
	}
	s.Git(dir, "config", "--add", "x.multi", "a")
	s.Git(dir, "config", "--add", "x.multi", "b c")
	if v, _ := g.ConfigGetAll("x.multi"); len(v) != 2 || v[1] != "b c" {
		t.Fatalf("get-all %v", v)
	}
	if err := g.RequireVersion(2, 32); err != nil {
		t.Fatal(err)
	}
	if err := g.RequireVersion(99, 0); err == nil {
		t.Fatal("RequireVersion(99) passed")
	}
}

// TestCommitTreeReproducesCommitsExactly is the foundation of `gids amend`:
// re-creating a commit with its own fields must yield the same object.
func TestCommitTreeReproducesCommitsExactly(t *testing.T) {
	s := testenv.New(t)
	dir := s.Repo("p", true)
	g := Client{Dir: dir}
	msgs := map[string]string{
		"normal":          "subject\n\nbody line\n\nCo-authored-by: A <a@x>\n",
		"no final nl":     "no newline at end",
		"unicode":         "Nguyễn: thêm tính năng 🎵\n",
		"leading blank":   "\n\nweird\n",
		"trailing spaces": "x   \n\n\n",
	}
	for name, msg := range msgs {
		t.Run(name, func(t *testing.T) {
			head := s.Git(dir, "rev-parse", "HEAD")
			tree := s.Git(dir, "rev-parse", "HEAD^{tree}")
			author := Ident{"Ann Author", "ann@x", "1700000000 -0800"}
			committer := Ident{"Cam Committer", "cam@x", "1700003600 +0530"}
			sha, err := g.CommitTree(tree, []string{head}, author, committer, "", msg)
			if err != nil {
				t.Fatal(err)
			}
			c, err := g.ReadCommit(sha)
			if err != nil {
				t.Fatal(err)
			}
			if c.Message != msg || c.Author != author || c.Committer != committer || c.Tree != tree || len(c.Parents) != 1 {
				t.Fatalf("read back %+v", c)
			}
			again, _ := g.CommitTree(c.Tree, c.Parents, c.Author, c.Committer, c.Encoding, c.Message)
			if again != sha {
				t.Fatalf("re-creating gave %s, want identical %s", again, sha)
			}
		})
	}
}

func TestReadCommitEncodingAndRoot(t *testing.T) {
	s := testenv.New(t)
	dir := s.Repo("p", false)
	g := Client{Dir: dir}
	os.WriteFile(filepath.Join(dir, "a"), []byte("a"), 0o644)
	s.Git(dir, "add", "a")
	s.Git(dir, "-c", "i18n.commitEncoding=ISO-8859-1", "-c", "user.name=A", "-c", "user.email=a@x",
		"commit", "-q", "--no-verify", "-m", "latin")
	c, err := g.ReadCommit("HEAD")
	if err != nil {
		t.Fatal(err)
	}
	if c.Encoding != "ISO-8859-1" || len(c.Parents) != 0 {
		t.Fatalf("commit %+v", c)
	}
	sha, err := g.CommitTree(c.Tree, nil, c.Author, c.Committer, c.Encoding, c.Message)
	if err != nil || sha != c.SHA {
		t.Fatalf("re-create root with encoding: %s vs %s (%v)", sha, c.SHA, err)
	}
	if _, err := g.ReadCommit("--output=/tmp/x"); err == nil {
		t.Fatal("option-looking revision accepted")
	}
}
