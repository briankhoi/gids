// Package ui is every interaction gids has with a person: styled messages,
// yes/no and text prompts, pickers and forms. In a real terminal they are
// Bubble Tea programs; otherwise (piped input, scripted answers, TERM=dumb)
// they fall back to plain numbered/line prompts. Both paths share the same
// key semantics (SPEC §7.2):
//
//	Ctrl-C        → ErrInterrupted ("stop everything")
//	Ctrl-D / Esc  → ErrSkipped     ("skip this question"); EOF is the same
package ui

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/charmbracelet/colorprofile"
)

var (
	// ErrInterrupted: the user pressed Ctrl-C (or the process got SIGINT/SIGTERM).
	ErrInterrupted = errors.New("interrupted")
	// ErrSkipped: the user pressed Ctrl-D or Esc, or input ended.
	ErrSkipped = errors.New("skipped")
)

// UI is one conversation with the user.
type UI struct {
	ctx    context.Context
	in     io.Reader
	rawOut io.Writer // for Bubble Tea, which does its own color detection
	out    io.Writer // colorprofile writer: strips color for non-terminals / NO_COLOR
	tty    bool      // full TUI allowed
	env    []string
	lines  chan lineResult // non-TTY line reader, started lazily
	S      Styles
}

// New builds a UI. The TUI is used only when both in and out are terminals
// and TERM isn't "dumb".
func New(ctx context.Context, in io.Reader, out io.Writer) *UI {
	env := os.Environ()
	return &UI{
		ctx:    ctx,
		in:     in,
		rawOut: out,
		out:    colorprofile.NewWriter(out, env),
		tty:    isTerminal(in) && isTerminal(out) && os.Getenv("TERM") != "dumb",
		env:    env,
		S:      DefaultStyles(),
	}
}

// Context is the UI's cancellation context (cancelled on Ctrl-C/SIGTERM).
func (u *UI) Context() context.Context { return u.ctx }

// Out is the styled output writer.
func (u *UI) Out() io.Writer { return u.out }

// Interactive reports whether the full TUI is in use.
func (u *UI) Interactive() bool { return u.tty }

func isTerminal(x any) bool {
	f, ok := x.(*os.File)
	if !ok {
		return false
	}
	fi, err := f.Stat()
	return err == nil && fi.Mode()&os.ModeCharDevice != 0
}

// Printf writes plain text.
func (u *UI) Printf(format string, a ...any) { fmt.Fprintf(u.out, format, a...) }

// Success, Warn, Fail and Hint write one styled line each.
func (u *UI) Success(format string, a ...any) { u.line(u.S.Success, "✓ ", format, a...) }
func (u *UI) Warn(format string, a ...any)    { u.line(u.S.Warn, "⚠ ", format, a...) }
func (u *UI) Fail(format string, a ...any)    { u.line(u.S.Error, "✗ ", format, a...) }
func (u *UI) Hint(format string, a ...any)    { u.line(u.S.Faint, "", format, a...) }

func (u *UI) line(st style, prefix, format string, a ...any) {
	fmt.Fprintln(u.out, st.Render(prefix+fmt.Sprintf(format, a...)))
}

// ---- non-TTY line input ----

type lineResult struct {
	line string
	err  error
}

// readLine returns the next input line (trimmed). EOF → ErrSkipped. A
// cancelled context (Ctrl-C) → ErrInterrupted, even while blocked reading.
func (u *UI) readLine() (string, error) {
	if u.lines == nil {
		u.lines = make(chan lineResult)
		go func() {
			r := bufio.NewReader(u.in)
			for {
				s, err := r.ReadString('\n')
				if err == io.EOF && s != "" {
					err = nil // final line without newline still counts
				}
				u.lines <- lineResult{strings.TrimRight(s, "\r\n"), err}
				if err != nil {
					return
				}
			}
		}()
	}
	select {
	case <-u.ctx.Done():
		return "", ErrInterrupted
	case r, ok := <-u.lines:
		if !ok || r.err != nil {
			u.lines = closedLines()
			return "", ErrSkipped
		}
		line := strings.TrimSpace(r.line)
		if line == "\x1b" { // a lone Esc typed into a cooked terminal
			return "", ErrSkipped
		}
		return line, nil
	}
}

// closedLines makes every later read report EOF immediately.
func closedLines() chan lineResult {
	c := make(chan lineResult)
	close(c)
	return c
}

func (u *UI) ask(prompt string) (string, error) {
	fmt.Fprint(u.out, prompt)
	s, err := u.readLine()
	if err != nil {
		fmt.Fprintln(u.out)
	}
	return s, err
}

// ---- prompts ----

// Confirm asks a yes/no question.
func (u *UI) Confirm(q string, def bool) (bool, error) {
	if u.tty {
		return u.confirmTUI(q, def)
	}
	opts := "[y/N]"
	if def {
		opts = "[Y/n]"
	}
	for {
		s, err := u.ask(q + " " + opts + ": ")
		if err != nil {
			return false, err
		}
		switch strings.ToLower(s) {
		case "":
			return def, nil
		case "y", "yes":
			return true, nil
		case "n", "no":
			return false, nil
		}
		u.Hint("Please answer y or n.")
	}
}

// Input asks for a line of text. def is used for an empty answer; validate
// (optional) rejects bad values, which are re-asked.
func (u *UI) Input(label, def string, validate func(string) error) (string, error) {
	if u.tty {
		return u.inputTUI(label, def, validate)
	}
	prompt := label + ": "
	if def != "" {
		prompt = fmt.Sprintf("%s [%s]: ", label, def)
	}
	for {
		s, err := u.ask(prompt)
		if err != nil {
			return "", err
		}
		if s == "" {
			s = def
		}
		if validate != nil {
			if err := validate(s); err != nil {
				u.Hint("%s", err)
				continue
			}
		}
		return s, nil
	}
}

// Item is one row of a picker.
type Item struct {
	Label  string // main text, also what filtering matches
	Detail string // secondary text (e.g. an email)
}

// Select lets the user pick one item and returns its index.
func (u *UI) Select(title string, items []Item) (int, error) {
	if len(items) == 0 {
		return -1, errors.New("nothing to choose from")
	}
	if u.tty {
		return u.selectTUI(title, items)
	}
	u.Printf("%s\n", title)
	for i, it := range items {
		u.Printf("  %d) %s\n", i+1, rowText(it))
	}
	for {
		s, err := u.ask(fmt.Sprintf("Enter choice (1-%d): ", len(items)))
		if err != nil {
			return -1, err
		}
		if n, ok := parseIndex(s, len(items)); ok {
			return n, nil
		}
		for i, it := range items { // also accept the label itself
			if strings.EqualFold(s, it.Label) {
				return i, nil
			}
		}
		u.Hint("Please enter a number between 1 and %d.", len(items))
	}
}

// MultiSelect lets the user check any subset; checked is the starting state.
func (u *UI) MultiSelect(title string, items []Item, checked []bool) ([]bool, error) {
	if u.tty {
		return u.multiTUI(title, items, checked)
	}
	cur := append([]bool(nil), checked...)
	u.Printf("%s\n", title)
	for i, it := range items {
		mark := " "
		if cur[i] {
			mark = "x"
		}
		u.Printf("  %d) [%s] %s\n", i+1, mark, rowText(it))
	}
	for {
		s, err := u.ask("Numbers to select (e.g. 1 3), Enter to keep, \"none\" to clear: ")
		if err != nil {
			return nil, err
		}
		switch strings.ToLower(s) {
		case "":
			return cur, nil
		case "none":
			return make([]bool, len(items)), nil
		}
		next, ok := make([]bool, len(items)), true
		for _, f := range strings.FieldsFunc(s, func(r rune) bool { return r == ' ' || r == ',' }) {
			n, valid := parseIndex(f, len(items))
			if !valid {
				ok = false
				break
			}
			next[n] = true
		}
		if ok {
			return next, nil
		}
		u.Hint("Please enter numbers between 1 and %d.", len(items))
	}
}

// Field is one form field.
type Field struct {
	Label    string
	Value    string                    // initial value
	Optional bool                      // empty is allowed
	Validate func(string) error        // optional
	Hint     func(value string) string // optional live hint (e.g. "✓ file exists")
}

// Form asks for several fields at once and returns their values.
func (u *UI) Form(title string, fields []Field) ([]string, error) {
	if u.tty {
		return u.formTUI(title, fields)
	}
	u.Printf("%s\n", title)
	vals := make([]string, len(fields))
	for i, f := range fields {
		label := f.Label
		if f.Optional {
			label += ` (optional; "none" clears)`
		}
		v, err := u.Input(label, f.Value, func(s string) error {
			if s == "none" && f.Optional {
				return nil
			}
			return checkField(f, s)
		})
		if err != nil {
			return nil, err
		}
		if v == "none" && f.Optional {
			v = ""
		}
		vals[i] = v
	}
	return vals, nil
}

func checkField(f Field, s string) error {
	if s == "" {
		if f.Optional {
			return nil
		}
		return fmt.Errorf("%s is required", f.Label)
	}
	if f.Validate != nil {
		return f.Validate(s)
	}
	return nil
}

func parseIndex(s string, n int) (int, bool) {
	var i int
	if _, err := fmt.Sscanf(s, "%d", &i); err != nil || fmt.Sprint(i) != s || i < 1 || i > n {
		return 0, false
	}
	return i - 1, true
}

func rowText(it Item) string {
	if it.Detail == "" {
		return it.Label
	}
	return it.Label + "  " + it.Detail
}
