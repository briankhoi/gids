package ui

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
)

// ---- key helpers ----

func key(s string) tea.KeyPressMsg {
	switch s {
	case "enter":
		return tea.KeyPressMsg{Code: tea.KeyEnter}
	case "esc":
		return tea.KeyPressMsg{Code: tea.KeyEscape}
	case "up":
		return tea.KeyPressMsg{Code: tea.KeyUp}
	case "down":
		return tea.KeyPressMsg{Code: tea.KeyDown}
	case "tab":
		return tea.KeyPressMsg{Code: tea.KeyTab}
	case "shift+tab":
		return tea.KeyPressMsg{Code: tea.KeyTab, Mod: tea.ModShift}
	case "space":
		return tea.KeyPressMsg{Code: tea.KeySpace, Text: " "}
	case "backspace":
		return tea.KeyPressMsg{Code: tea.KeyBackspace}
	case "pgdown":
		return tea.KeyPressMsg{Code: tea.KeyPgDown}
	}
	if c, ok := strings.CutPrefix(s, "ctrl+"); ok {
		return tea.KeyPressMsg{Code: rune(c[0]), Mod: tea.ModCtrl}
	}
	return tea.KeyPressMsg{Code: rune(s[0]), Text: s}
}

// press feeds keys to a model; "type:abc" types text.
func press[M tea.Model](m M, keys ...string) M {
	var cur tea.Model = m
	for _, k := range keys {
		if t, ok := strings.CutPrefix(k, "type:"); ok {
			for _, r := range t {
				cur, _ = cur.Update(key(string(r)))
			}
			continue
		}
		cur, _ = cur.Update(key(k))
	}
	return cur.(M)
}

func items(labels ...string) []Item {
	var out []Item
	for _, l := range labels {
		out = append(out, Item{Label: l, Detail: strings.ToLower(l) + "@x.com"})
	}
	return out
}

// ---- confirm ----

func TestConfirmKeys(t *testing.T) {
	cases := []struct {
		keys   []string
		def    bool
		answer bool
		o      outcome
	}{
		{[]string{"y"}, false, true, done},
		{[]string{"Y"}, false, true, done},
		{[]string{"n"}, true, false, done},
		{[]string{"enter"}, true, true, done},
		{[]string{"enter"}, false, false, done},
		{[]string{"x", "q", "y"}, false, true, done}, // unrelated keys ignored
		{[]string{"ctrl+c"}, true, false, interrupted},
		{[]string{"ctrl+d"}, true, false, skipped},
		{[]string{"esc"}, true, false, skipped},
	}
	for _, tc := range cases {
		m := press(confirmModel{q: "Sure?", def: tc.def, s: DefaultStyles()}, tc.keys...)
		if m.o != tc.o || (m.o == done && m.answer != tc.answer) {
			t.Errorf("keys %v def=%v: outcome %v answer %v", tc.keys, tc.def, m.o, m.answer)
		}
		if !strings.Contains(m.View().Content, "Sure?") {
			t.Errorf("view lost the question: %q", m.View().Content)
		}
	}
}

// ---- input ----

func TestInputValidationAndDefaults(t *testing.T) {
	notEmpty := func(s string) error {
		if s == "" {
			return errors.New("required")
		}
		return nil
	}
	m := press(newInput("Name", "", notEmpty, DefaultStyles()), "enter")
	if m.o != running || !strings.Contains(m.View().Content, "required") {
		t.Fatalf("empty input accepted: %v %q", m.o, m.View().Content)
	}
	m = press(m, "type:Bob", "enter")
	if m.o != done || m.value() != "Bob" {
		t.Fatalf("got %v %q", m.o, m.value())
	}
	m = press(newInput("Name", "Default", nil, DefaultStyles()), "enter")
	if m.value() != "Default" {
		t.Fatalf("default not used: %q", m.value())
	}
	// Ctrl-D with text deletes (stays running); on empty input it skips.
	m = press(newInput("Name", "", nil, DefaultStyles()), "type:ab", "ctrl+d")
	if m.o != running {
		t.Fatalf("ctrl+d with text should not skip: %v", m.o)
	}
	m = press(newInput("Name", "", nil, DefaultStyles()), "ctrl+d")
	if m.o != skipped {
		t.Fatalf("ctrl+d on empty: %v", m.o)
	}
	if press(newInput("Name", "", nil, DefaultStyles()), "type:x", "ctrl+c").o != interrupted {
		t.Fatal("ctrl+c should interrupt")
	}
}

// ---- pickers ----

func TestSelectMoveFilterAndChoose(t *testing.T) {
	its := items("Work", "Personal", "School")
	m := press(newPick("Pick", its, false, nil, DefaultStyles()), "down", "down", "down", "enter") // clamps at last
	if m.o != done || m.choice != 2 {
		t.Fatalf("choice %d %v", m.choice, m.o)
	}
	m = press(newPick("Pick", its, false, nil, DefaultStyles()), "up", "enter") // clamps at first
	if m.choice != 0 {
		t.Fatalf("choice %d", m.choice)
	}
	m = press(newPick("Pick", its, false, nil, DefaultStyles()), "type:sch", "enter")
	if m.choice != 2 {
		t.Fatalf("filter 'sch' chose %d", m.choice)
	}
	m = press(newPick("Pick", its, false, nil, DefaultStyles()), "type:personal@", "enter") // matches detail too
	if m.choice != 1 {
		t.Fatalf("filter by detail chose %d", m.choice)
	}
	m = press(newPick("Pick", its, false, nil, DefaultStyles()), "type:zzz", "enter")
	if m.o != running || !strings.Contains(m.View().Content, "no matches") {
		t.Fatalf("enter with no matches must not select: %v", m.o)
	}
	m = press(m, "esc") // first Esc clears the filter
	if m.o != running || len(m.shown) != 3 {
		t.Fatalf("esc should clear filter first: %v %d", m.o, len(m.shown))
	}
	if press(m, "esc").o != skipped {
		t.Fatal("second esc should skip")
	}
	if press(newPick("Pick", its, false, nil, DefaultStyles()), "ctrl+c").o != interrupted {
		t.Fatal("ctrl+c")
	}
	if press(newPick("Pick", its, false, nil, DefaultStyles()), "ctrl+d").o != skipped {
		t.Fatal("ctrl+d on empty filter should skip")
	}
	// ctrl+n / ctrl+p move too
	if m := press(newPick("Pick", its, false, nil, DefaultStyles()), "ctrl+n", "ctrl+n", "ctrl+p", "enter"); m.choice != 1 {
		t.Fatalf("ctrl+n/p: %d", m.choice)
	}
}

// D7: every key that works is shown in the header, so none needs discovering.
func TestPickerHintShowsEveryKeyThatWorks(t *testing.T) {
	head := func(m pickModel) string {
		v := stripANSI(m.View().Content)
		return v[:strings.Index(v, "/ ")] // everything above the filter line
	}
	its := []Item{{Label: "Work"}, {Label: "Personal"}}
	h := head(newPick("Pick", its, false, nil, DefaultStyles()))
	for _, want := range []string{"ctrl+p/n", "enter select", "type to filter", "esc cancel"} {
		if !strings.Contains(h, want) {
			t.Errorf("hint lacks %q: %q", want, h)
		}
	}
	if strings.Contains(h, "pgup") {
		t.Errorf("paging shown for a list that fits: %q", h)
	}
	if h := head(press(newPick("Pick", its, false, nil, DefaultStyles()), "type:wo")); !strings.Contains(h, "esc clear filter") || strings.Contains(h, "esc cancel") {
		t.Errorf("with a filter, esc must say it clears: %q", h)
	}
	if h := head(newPick("Pick", its, true, nil, DefaultStyles())); !strings.Contains(h, "space toggle") || !strings.Contains(h, "enter save") {
		t.Errorf("multi-select hint: %q", h)
	}
	var many []Item
	for i := range 30 {
		many = append(many, Item{Label: fmt.Sprint("p", i)})
	}
	var m tea.Model = newPick("A title", many, false, nil, DefaultStyles())
	m, _ = m.Update(tea.WindowSizeMsg{Width: 200, Height: 8})
	if h := head(m.(pickModel)); !strings.Contains(h, "pgup/pgdn page") {
		t.Errorf("overflowing list must show paging: %q", h)
	}
	// Too narrow for title + hint on one line: hint wraps to its own line, intact.
	m, _ = m.Update(tea.WindowSizeMsg{Width: 90, Height: 8})
	lines := strings.Split(strings.TrimSpace(head(m.(pickModel))), "\n")
	if len(lines) != 2 || lines[0] != "A title" || !strings.HasSuffix(lines[1], "esc cancel)") {
		t.Errorf("hint should move below the title, untruncated: %q", lines)
	}
}

func TestSelectScrollsAndTruncatesOnSmallTerminals(t *testing.T) {
	var its []Item
	for i := range 40 {
		its = append(its, Item{Label: strings.Repeat("x", 100) + string(rune('A'+i%26)), Detail: "d"})
	}
	var m tea.Model = newPick("Pick", its, false, nil, DefaultStyles())
	m, _ = m.Update(tea.WindowSizeMsg{Width: 30, Height: 8})
	pm := press(m.(pickModel), "pgdown", "pgdown", "down")
	view := pm.View().Content
	for _, line := range strings.Split(view, "\n") {
		if w := len([]rune(stripANSI(line))); w > 29 {
			t.Fatalf("row wider than terminal (%d): %q", w, line)
		}
	}
	if !strings.Contains(view, "of 40") {
		t.Fatalf("no scroll indicator: %q", view)
	}
	if pm.cursor < pm.offset || pm.cursor >= pm.offset+pm.rows() {
		t.Fatalf("cursor %d not visible (offset %d rows %d)", pm.cursor, pm.offset, pm.rows())
	}
}

func stripANSI(s string) string {
	var b strings.Builder
	esc := false
	for _, r := range s {
		switch {
		case r == 0x1b:
			esc = true
		case esc && (r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z'):
			esc = false
		case !esc:
			b.WriteRune(r)
		}
	}
	return b.String()
}

func TestMultiSelectToggle(t *testing.T) {
	its := items("Alice", "Bob", "Carol")
	m := press(newPick("Who", its, true, []bool{true, false, false}, DefaultStyles()), "space", "down", "space", "down", "down", "space", "enter")
	if m.o != done || m.checked[0] || !m.checked[1] || !m.checked[2] {
		t.Fatalf("checked %v %v", m.checked, m.o)
	}
	// Toggling while filtered acts on the filtered row.
	m = press(newPick("Who", its, true, nil, DefaultStyles()), "type:car", "space", "enter")
	if !m.checked[2] || m.checked[0] {
		t.Fatalf("filtered toggle: %v", m.checked)
	}
	// Space can't be typed into the filter in multi mode, but enter with zero checked is fine.
	if m := press(newPick("Who", its, true, nil, DefaultStyles()), "enter"); m.o != done {
		t.Fatal("empty selection should be allowed")
	}
}

// ---- form ----

func TestFormNavigationAndValidation(t *testing.T) {
	email := func(s string) error {
		if !strings.Contains(s, "@") {
			return errors.New("bad email")
		}
		return nil
	}
	fields := []Field{{Label: "Name"}, {Label: "Email", Validate: email}, {Label: "Key", Optional: true}}
	m := press(newForm("New", fields, DefaultStyles()), "type:Bob", "tab", "type:nope", "tab", "enter")
	if m.o != running || m.focus != 1 || m.errs[1] == "" {
		t.Fatalf("invalid email should refocus it: o=%v focus=%d errs=%v", m.o, m.focus, m.errs)
	}
	m = press(m, "backspace", "backspace", "backspace", "backspace", "type:b@x", "enter", "enter")
	if m.o != done {
		t.Fatalf("form not done: %v %v", m.o, m.errs)
	}
	if v := m.values(); v[0] != "Bob" || v[1] != "b@x" || v[2] != "" {
		t.Fatalf("values %q", v)
	}
	// Required field empty → error on submit; shift+tab wraps around.
	m = press(newForm("New", fields, DefaultStyles()), "shift+tab", "enter")
	if m.o != running || m.errs[0] == "" {
		t.Fatalf("required check: %v %v", m.o, m.errs)
	}
	if press(newForm("New", fields, DefaultStyles()), "esc").o != skipped {
		t.Fatal("esc")
	}
	if press(newForm("New", fields, DefaultStyles()), "type:x", "ctrl+c").o != interrupted {
		t.Fatal("ctrl+c")
	}
	hint := []Field{{Label: "Key", Hint: func(v string) string { return "len=" + string(rune('0'+len(v))) }}}
	if v := press(newForm("H", hint, DefaultStyles()), "type:abc").View().Content; !strings.Contains(v, "len=3") {
		t.Fatalf("live hint missing: %q", v)
	}
}

// ---- line fallback (non-terminal) ----

func lineUI(input string) (*UI, *bytes.Buffer) {
	var out bytes.Buffer
	return New(context.Background(), strings.NewReader(input), &out), &out
}

func TestFallbackConfirm(t *testing.T) {
	cases := []struct {
		in   string
		def  bool
		want bool
		err  error
	}{
		{"y\n", false, true, nil}, {"YES\n", false, true, nil}, {"n\n", true, false, nil},
		{"\n", true, true, nil}, {"maybe\nno\n", true, false, nil},
		{"", true, false, ErrSkipped}, {"maybe\n", true, false, ErrSkipped}, // EOF ends the loop (B1)
		{"y", false, true, nil}, // last line without newline
		{"\x1b\n", true, false, ErrSkipped},
	}
	for _, tc := range cases {
		u, _ := lineUI(tc.in)
		got, err := u.Confirm("Q?", tc.def)
		if got != tc.want || !errors.Is(err, tc.err) && !(err == nil && tc.err == nil) {
			t.Errorf("Confirm(%q, def=%v) = %v, %v; want %v, %v", tc.in, tc.def, got, err, tc.want, tc.err)
		}
	}
}

func TestFallbackInputNeverLoopsForever(t *testing.T) {
	u, out := lineUI("\n\n\n")
	_, err := u.Input("Name", "", func(s string) error {
		if s == "" {
			return errors.New("required")
		}
		return nil
	})
	if !errors.Is(err, ErrSkipped) {
		t.Fatalf("err = %v", err)
	}
	if n := strings.Count(out.String(), "required"); n != 3 {
		t.Fatalf("asked %d times, want 3 then stop at EOF", n)
	}
}

func TestFallbackSelectAndMulti(t *testing.T) {
	u, _ := lineUI("0\n4\nabc\n2\n")
	if i, err := u.Select("Pick", items("A", "B", "C")); i != 1 || err != nil {
		t.Fatalf("Select = %d %v", i, err)
	}
	u, _ = lineUI("personal\n")
	if i, _ := u.Select("Pick", items("Work", "Personal")); i != 1 {
		t.Fatalf("select by label = %d", i)
	}
	u, _ = lineUI("1 3\n")
	if got, _ := u.MultiSelect("M", items("A", "B", "C"), []bool{false, true, false}); !got[0] || got[1] || !got[2] {
		t.Fatalf("multi %v", got)
	}
	u, _ = lineUI("\n")
	if got, _ := u.MultiSelect("M", items("A", "B"), []bool{false, true}); got[0] || !got[1] {
		t.Fatalf("enter keeps current: %v", got)
	}
	u, _ = lineUI("none\n")
	if got, _ := u.MultiSelect("M", items("A", "B"), []bool{true, true}); got[0] || got[1] {
		t.Fatalf("none clears: %v", got)
	}
	u, _ = lineUI("1,9\n2\n")
	if got, _ := u.MultiSelect("M", items("A", "B"), []bool{false, false}); got[0] || !got[1] {
		t.Fatalf("invalid list should re-ask: %v", got)
	}
	u, _ = lineUI("")
	if _, err := u.Select("Pick", nil); err == nil {
		t.Fatal("empty select should error")
	}
}

func TestFallbackForm(t *testing.T) {
	fields := []Field{{Label: "Name", Value: "Old"}, {Label: "Key", Value: "~/k", Optional: true}, {Label: "Email"}}
	u, _ := lineUI("\nnone\na@b\n")
	vals, err := u.Form("Edit", fields)
	if err != nil || vals[0] != "Old" || vals[1] != "" || vals[2] != "a@b" {
		t.Fatalf("form %q %v", vals, err)
	}
	u, _ = lineUI("x\n") // ends early
	if _, err := u.Form("Edit", fields[2:]); err != nil {
		t.Fatal(err)
	}
	u, _ = lineUI("")
	if _, err := u.Form("Edit", fields[2:]); !errors.Is(err, ErrSkipped) {
		t.Fatalf("form EOF: %v", err)
	}
}

// blockingReader never returns, like a terminal nobody types into.
type blockingReader struct{}

func (blockingReader) Read([]byte) (int, error) { select {} }

func TestFallbackCtrlCWhileBlocked(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	u := New(ctx, blockingReader{}, io.Discard)
	go func() { time.Sleep(20 * time.Millisecond); cancel() }()
	if _, err := u.Confirm("Q?", true); !errors.Is(err, ErrInterrupted) {
		t.Fatalf("err = %v, want ErrInterrupted", err)
	}
}

func TestNonTerminalIsNotInteractive(t *testing.T) {
	u, _ := lineUI("")
	if u.Interactive() {
		t.Fatal("buffers must not be treated as a terminal")
	}
}

func TestStyledOutputHasNoColorForNonTerminals(t *testing.T) {
	u, out := lineUI("")
	u.Success("done %d", 1)
	u.Warn("careful")
	if strings.Contains(out.String(), "\x1b[") {
		t.Fatalf("ANSI codes written to a non-terminal: %q", out.String())
	}
	if !strings.Contains(out.String(), "✓ done 1") || !strings.Contains(out.String(), "⚠ careful") {
		t.Fatalf("output %q", out.String())
	}
}
