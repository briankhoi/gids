package render

import (
	"errors"
	"strings"
	"testing"
)

const body = "[include]\n\tpath = \"~/g\"\n"

func TestBlockRoundTripIsByteExact(t *testing.T) {
	cases := map[string]struct {
		content string
		exists  bool
	}{
		"missing file":           {"", false},
		"empty file":             {"", true},
		"ends with newline":      {"[user]\n\tname = A\n", true},
		"no final newline":       {"[user]\n\tname = A", true},
		"only newline":           {"\n", true},
		"crlf":                   {"[user]\r\n\tname = A\r\n", true},
		"crlf no final":          {"[user]\r\n\tname = A", true},
		"unicode and blank tail": {"# héllo\n\n\n", true},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			with, err := WithBlock(tc.content, body, tc.exists)
			if err != nil {
				t.Fatal(err)
			}
			if !strings.HasSuffix(with, endMarker+"\n") || !strings.Contains(with, body) {
				t.Fatalf("block not appended last:\n%q", with)
			}
			if ok, err := HasBlock(with); !ok || err != nil {
				t.Fatalf("HasBlock = %v %v", ok, err)
			}
			again, err := WithBlock(with, body, true) // idempotent
			if err != nil || again != with {
				t.Fatalf("second WithBlock changed content:\n%q\n%q", with, again)
			}
			out, remove, err := WithoutBlock(with)
			if err != nil {
				t.Fatal(err)
			}
			if !tc.exists {
				if !remove {
					t.Fatal("file gids created should be removed")
				}
				return
			}
			if remove || out != tc.content {
				t.Fatalf("not restored: got %q (remove=%v), want %q", out, remove, tc.content)
			}
		})
	}
}

func TestWithBlockUpdatesBodyInPlace(t *testing.T) {
	with, _ := WithBlock("[user]\n", body, true)
	updated, err := WithBlock(with, "[include]\n\tpath = \"/new\"\n", true)
	if err != nil || strings.Count(updated, "# gids:begin") != 1 || !strings.Contains(updated, "/new") {
		t.Fatalf("update: %v\n%s", err, updated)
	}
}

func TestWithBlockMovesBlockAfterLaterUserSettings(t *testing.T) {
	with, _ := WithBlock("[user]\n\tname = A\n", body, true)
	with += "[user]\n\temail = later@x\n" // e.g. `git config --global user.email` appended a section
	moved, err := WithBlock(with, body, true)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Index(moved, "later@x") > strings.Index(moved, "# gids:begin") {
		t.Fatalf("block should move after the user's later settings:\n%s", moved)
	}
	out, _, _ := WithoutBlock(moved)
	if out != "[user]\n\tname = A\n[user]\n\temail = later@x\n" {
		t.Fatalf("restore after move: %q", out)
	}
}

func TestUninstallKeepsLaterUserEdits(t *testing.T) {
	with, _ := WithBlock("[user]\n\tname = A", body, true) // gids added a newline
	with += "[alias]\n\tco = checkout\n"                   // user added after
	out, remove, err := WithoutBlock(with)
	if err != nil || remove {
		t.Fatal(err, remove)
	}
	if out != "[user]\n\tname = A\n[alias]\n\tco = checkout\n" {
		t.Fatalf("got %q", out)
	}
}

func TestMalformedBlocksAreRefused(t *testing.T) {
	b, _ := WithBlock("", body, true)
	cases := map[string]string{
		"begin without end": beginMarker + "\n" + body,
		"end without begin": "[x]\n" + endMarker + "\n",
		"two blocks":        b + b,
		"nested":            beginMarker + "\n" + beginMarker + "\n" + endMarker + "\n",
		"altered begin":     "# gids:begin edited\n" + endMarker + "\n",
		"altered end":       beginMarker + "\n# gids:end!\n",
	}
	for name, content := range cases {
		t.Run(name, func(t *testing.T) {
			if _, err := WithBlock(content, body, true); !errors.Is(err, ErrMalformedBlock) {
				t.Errorf("WithBlock err = %v", err)
			}
			if _, _, err := WithoutBlock(content); !errors.Is(err, ErrMalformedBlock) {
				t.Errorf("WithoutBlock err = %v", err)
			}
		})
	}
}

func TestMarkerTextInsideALineIsNotAMarker(t *testing.T) {
	content := "[alias]\n\tx = !echo '# gids:begin'\n"
	if ok, err := HasBlock(content); ok || err != nil {
		t.Fatalf("HasBlock = %v %v", ok, err)
	}
}

// FuzzBlockRoundTrip: for any file without gids markers, install then
// uninstall restores it byte-for-byte (SPEC §11 I3/I8).
func FuzzBlockRoundTrip(f *testing.F) {
	for _, s := range []string{"", "\n", "[user]\n\tname = A", "a\r\nb", "x\n\n"} {
		f.Add(s, true)
	}
	f.Add("", false)
	f.Fuzz(func(t *testing.T, content string, exists bool) {
		if strings.Contains(content, "# gids:") || (!exists && content != "") {
			t.Skip()
		}
		with, err := WithBlock(content, body, exists)
		if err != nil {
			t.Fatalf("WithBlock(%q): %v", content, err)
		}
		if !strings.HasPrefix(with, content) {
			t.Fatalf("WithBlock changed existing content: %q -> %q", content, with)
		}
		out, remove, err := WithoutBlock(with)
		if err != nil {
			t.Fatal(err)
		}
		if exists && (remove || out != content) {
			t.Fatalf("round trip: %q -> %q (remove=%v)", content, out, remove)
		}
		if !exists && !remove {
			t.Fatalf("created file not marked for removal")
		}
	})
}
