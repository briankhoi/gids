package ui

import (
	"errors"
	"fmt"
	"strings"

	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
)

// outcome is how a TUI model ended.
type outcome int

const (
	running outcome = iota
	done
	skipped
	interrupted
)

func (o outcome) err() error {
	switch o {
	case done:
		return nil
	case skipped:
		return ErrSkipped
	default: // interrupted, or killed while still running
		return ErrInterrupted
	}
}

// commonKey handles the keys every model treats the same. It reports whether
// the key was consumed.
func commonKey(k string, o *outcome) (tea.Cmd, bool) {
	switch k {
	case "ctrl+c":
		*o = interrupted
		return tea.Quit, true
	case "esc", "ctrl+d":
		*o = skipped
		return tea.Quit, true
	case "ctrl+z":
		return tea.Suspend, true
	}
	return nil, false
}

// run executes a model inline (no alternate screen) on the UI's terminal.
func (u *UI) run(m tea.Model) (tea.Model, error) {
	p := tea.NewProgram(m,
		tea.WithInput(u.in), tea.WithOutput(u.rawOut),
		tea.WithContext(u.ctx), tea.WithEnvironment(u.env))
	fm, err := p.Run()
	if err != nil && !errors.Is(err, tea.ErrInterrupted) && !errors.Is(err, tea.ErrProgramKilled) {
		return fm, err
	}
	if err != nil {
		return fm, ErrInterrupted
	}
	return fm, nil
}

// ---------------- confirm ----------------

type confirmModel struct {
	q      string
	def    bool
	answer bool
	o      outcome
	s      Styles
}

func (m confirmModel) Init() tea.Cmd { return nil }

func (m confirmModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	k, ok := msg.(tea.KeyPressMsg)
	if !ok {
		return m, nil
	}
	if cmd, used := commonKey(k.String(), &m.o); used {
		return m, cmd
	}
	switch strings.ToLower(k.String()) {
	case "y":
		m.answer, m.o = true, done
	case "n":
		m.answer, m.o = false, done
	case "enter":
		m.answer, m.o = m.def, done
	default:
		return m, nil
	}
	return m, tea.Quit
}

func (m confirmModel) View() tea.View {
	opts := "[y/N]"
	if m.def {
		opts = "[Y/n]"
	}
	s := m.q + " " + m.s.Faint.Render(opts) + " "
	switch m.o {
	case done:
		if m.answer {
			s += "yes"
		} else {
			s += "no"
		}
		s += "\n"
	case skipped:
		s += m.s.Faint.Render("skipped") + "\n"
	case interrupted:
		s += m.s.Faint.Render("cancelled") + "\n"
	}
	return tea.NewView(s)
}

func (u *UI) confirmTUI(q string, def bool) (bool, error) {
	fm, err := u.run(confirmModel{q: q, def: def, s: u.S})
	if err != nil {
		return false, err
	}
	m := fm.(confirmModel)
	return m.answer, m.o.err()
}

// ---------------- text input ----------------

type inputModel struct {
	label    string
	ti       textinput.Model
	def      string
	validate func(string) error
	errMsg   string
	o        outcome
	s        Styles
}

func newInput(label, def string, validate func(string) error, s Styles) inputModel {
	ti := textinput.New()
	ti.Prompt = ""
	ti.Placeholder = def
	ti.Focus()
	return inputModel{label: label, ti: ti, def: def, validate: validate, s: s}
}

func (m inputModel) Init() tea.Cmd { return nil }

func (m inputModel) value() string {
	v := strings.TrimSpace(m.ti.Value())
	if v == "" {
		return m.def
	}
	return v
}

func (m inputModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	if k, ok := msg.(tea.KeyPressMsg); ok {
		key := k.String()
		// Ctrl-D deletes forward inside text; it only means "skip" when empty.
		if key == "ctrl+d" && m.ti.Value() != "" {
			key = ""
		}
		if key != "" {
			if cmd, used := commonKey(key, &m.o); used {
				return m, cmd
			}
		}
		if key == "enter" {
			if m.validate != nil {
				if err := m.validate(m.value()); err != nil {
					m.errMsg = err.Error()
					return m, nil
				}
			}
			m.o = done
			return m, tea.Quit
		}
		m.errMsg = ""
	}
	var cmd tea.Cmd
	m.ti, cmd = m.ti.Update(msg)
	return m, cmd
}

func (m inputModel) View() tea.View {
	s := m.label + ": "
	if m.o == done {
		return tea.NewView(s + m.value() + "\n")
	}
	if m.o != running {
		return tea.NewView(s + m.s.Faint.Render("cancelled") + "\n")
	}
	s += m.ti.View()
	if m.errMsg != "" {
		s += "\n" + m.s.Error.Render("  "+m.errMsg)
	}
	return tea.NewView(s)
}

func (u *UI) inputTUI(label, def string, validate func(string) error) (string, error) {
	fm, err := u.run(newInput(label, def, validate, u.S))
	if err != nil {
		return "", err
	}
	m := fm.(inputModel)
	return m.value(), m.o.err()
}

// ---------------- pickers (single and multi) ----------------

const maxRows = 10

type pickModel struct {
	title   string
	items   []Item
	multi   bool
	checked []bool
	filter  textinput.Model
	shown   []int // indexes of items matching the filter
	cursor  int   // index into shown
	offset  int   // first visible row of shown
	width   int
	height  int
	choice  int
	o       outcome
	s       Styles
}

func newPick(title string, items []Item, multi bool, checked []bool, s Styles) pickModel {
	f := textinput.New()
	f.Prompt = "/ "
	f.Placeholder = "type to filter"
	f.Focus()
	m := pickModel{title: title, items: items, multi: multi, filter: f, choice: -1, s: s, width: 80, height: 24}
	if multi {
		m.checked = append([]bool(nil), checked...)
		if len(m.checked) < len(items) {
			m.checked = append(m.checked, make([]bool, len(items)-len(m.checked))...)
		}
	}
	m.refilter()
	return m
}

func (m *pickModel) refilter() {
	q := strings.ToLower(strings.TrimSpace(m.filter.Value()))
	m.shown = m.shown[:0]
	for i, it := range m.items {
		if q == "" || strings.Contains(strings.ToLower(it.Label+" "+it.Detail), q) {
			m.shown = append(m.shown, i)
		}
	}
	if m.cursor >= len(m.shown) {
		m.cursor = max(0, len(m.shown)-1)
	}
	m.scroll()
}

// keyHint lists every key that does something right now, so none has to be
// discovered by accident (paging only when the list overflows; esc says what
// it will do, since it clears a filter before it cancels).
func (m *pickModel) keyHint() string {
	keys := []string{"↑/↓ or ctrl+p/n move"}
	if len(m.shown) > m.rows() {
		keys = append(keys, "pgup/pgdn page")
	}
	if m.multi {
		keys = append(keys, "space toggle", "enter save")
	} else {
		keys = append(keys, "enter select")
	}
	if m.filter.Value() != "" {
		keys = append(keys, "esc clear filter")
	} else {
		keys = append(keys, "type to filter", "esc cancel")
	}
	return strings.Join(keys, " · ")
}

func (m *pickModel) rows() int {
	return max(1, min(maxRows, m.height-4))
}

func (m *pickModel) scroll() {
	if m.cursor < m.offset {
		m.offset = m.cursor
	}
	if m.cursor >= m.offset+m.rows() {
		m.offset = m.cursor - m.rows() + 1
	}
	m.offset = max(0, min(m.offset, max(0, len(m.shown)-m.rows())))
}

func (m pickModel) Init() tea.Cmd { return nil }

func (m pickModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		m.scroll()
		return m, nil
	case tea.KeyPressMsg:
		key := msg.String()
		if key == "esc" && m.filter.Value() != "" { // Esc first clears the filter
			m.filter.SetValue("")
			m.refilter()
			return m, nil
		}
		if key == "ctrl+d" && m.filter.Value() != "" {
			key = "" // forward-delete in the filter
		}
		if key != "" {
			if cmd, used := commonKey(key, &m.o); used {
				return m, cmd
			}
		}
		switch key {
		case "up", "ctrl+p":
			if m.cursor > 0 {
				m.cursor--
			}
			m.scroll()
			return m, nil
		case "down", "ctrl+n":
			if m.cursor < len(m.shown)-1 {
				m.cursor++
			}
			m.scroll()
			return m, nil
		case "pgup":
			m.cursor = max(0, m.cursor-m.rows())
			m.scroll()
			return m, nil
		case "pgdown":
			m.cursor = max(0, min(len(m.shown)-1, m.cursor+m.rows()))
			m.scroll()
			return m, nil
		case "space":
			if m.multi && len(m.shown) > 0 {
				i := m.shown[m.cursor]
				m.checked[i] = !m.checked[i]
				return m, nil
			}
		case "enter":
			if m.multi {
				m.o = done
				return m, tea.Quit
			}
			if len(m.shown) > 0 {
				m.choice, m.o = m.shown[m.cursor], done
				return m, tea.Quit
			}
			return m, nil
		}
	}
	var cmd tea.Cmd
	before := m.filter.Value()
	m.filter, cmd = m.filter.Update(msg)
	if m.filter.Value() != before {
		m.refilter()
	}
	return m, cmd
}

func (m pickModel) View() tea.View {
	var b strings.Builder
	// The key hint goes beside the title if it fits, else on its own line:
	// it must stay readable, not be truncated away.
	w := max(10, m.width-1)
	title, hint := m.s.Title.Render(m.title), m.s.Faint.Render("("+m.keyHint()+")")
	if ansi.StringWidth(title)+2+ansi.StringWidth(hint) <= w {
		b.WriteString(title + "  " + hint + "\n")
	} else {
		b.WriteString(title + "\n" + ansi.Truncate(hint, w, "…") + "\n")
	}
	if m.o != running {
		return tea.NewView(m.finalView())
	}
	b.WriteString(m.filter.View() + "\n")
	if len(m.shown) == 0 {
		b.WriteString(m.s.Faint.Render("  no matches") + "\n")
	}
	end := min(len(m.shown), m.offset+m.rows())
	for r := m.offset; r < end; r++ {
		i := m.shown[r]
		prefix := "  "
		if r == m.cursor {
			prefix = m.s.Cursor.Render("▸ ")
		}
		box := ""
		if m.multi {
			box = "[ ] "
			if m.checked[i] {
				box = "[x] "
			}
		}
		line := prefix + box + m.items[i].Label
		if d := m.items[i].Detail; d != "" {
			line += "  " + m.s.Faint.Render(d)
		}
		b.WriteString(ansi.Truncate(line, max(10, m.width-1), "…") + "\n")
	}
	if len(m.shown) > m.rows() {
		b.WriteString(m.s.Faint.Render(fmt.Sprintf("  %d–%d of %d", m.offset+1, end, len(m.shown))) + "\n")
	}
	return tea.NewView(b.String())
}

func (m pickModel) finalView() string {
	head := m.title + ": "
	switch m.o {
	case done:
		if !m.multi {
			return head + m.items[m.choice].Label + "\n"
		}
		var names []string
		for i, c := range m.checked {
			if c {
				names = append(names, m.items[i].Label)
			}
		}
		if len(names) == 0 {
			return head + m.s.Faint.Render("none") + "\n"
		}
		return head + strings.Join(names, ", ") + "\n"
	default:
		return head + m.s.Faint.Render("cancelled") + "\n"
	}
}

func (u *UI) selectTUI(title string, items []Item) (int, error) {
	fm, err := u.run(newPick(title, items, false, nil, u.S))
	if err != nil {
		return -1, err
	}
	m := fm.(pickModel)
	return m.choice, m.o.err()
}

func (u *UI) multiTUI(title string, items []Item, checked []bool) ([]bool, error) {
	fm, err := u.run(newPick(title, items, true, checked, u.S))
	if err != nil {
		return nil, err
	}
	m := fm.(pickModel)
	return m.checked, m.o.err()
}

// ---------------- form ----------------

type formModel struct {
	title  string
	fields []Field
	inputs []textinput.Model
	focus  int
	errs   []string
	o      outcome
	s      Styles
}

func newForm(title string, fields []Field, s Styles) formModel {
	m := formModel{title: title, fields: fields, errs: make([]string, len(fields)), s: s}
	for i, f := range fields {
		ti := textinput.New()
		ti.Prompt = ""
		ti.SetValue(f.Value)
		if f.Optional {
			ti.Placeholder = "(optional)"
		}
		if i == 0 {
			ti.Focus()
		}
		m.inputs = append(m.inputs, ti)
	}
	return m
}

func (m formModel) Init() tea.Cmd { return nil }

func (m *formModel) move(d int) tea.Cmd {
	m.inputs[m.focus].Blur()
	m.focus = (m.focus + d + len(m.inputs)) % len(m.inputs)
	return m.inputs[m.focus].Focus()
}

// validateAll checks every field, records errors, and focuses the first bad one.
func (m *formModel) validateAll() bool {
	first := -1
	for i, f := range m.fields {
		m.errs[i] = ""
		if err := checkField(f, strings.TrimSpace(m.inputs[i].Value())); err != nil {
			m.errs[i] = err.Error()
			if first < 0 {
				first = i
			}
		}
	}
	if first >= 0 {
		m.move(first - m.focus)
		return false
	}
	return true
}

func (m formModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	if k, ok := msg.(tea.KeyPressMsg); ok {
		key := k.String()
		if key == "ctrl+d" && m.inputs[m.focus].Value() != "" {
			key = ""
		}
		if key != "" {
			if cmd, used := commonKey(key, &m.o); used {
				return m, cmd
			}
		}
		switch key {
		case "tab", "down":
			return m, m.move(1)
		case "shift+tab", "up":
			return m, m.move(-1)
		case "enter":
			if m.focus < len(m.inputs)-1 {
				return m, m.move(1)
			}
			if m.validateAll() {
				m.o = done
				return m, tea.Quit
			}
			return m, nil
		}
		m.errs[m.focus] = ""
	}
	var cmd tea.Cmd
	m.inputs[m.focus], cmd = m.inputs[m.focus].Update(msg)
	return m, cmd
}

func (m formModel) View() tea.View {
	var b strings.Builder
	b.WriteString(m.s.Title.Render(m.title) + "  " + m.s.Faint.Render("(tab/↑↓ move · enter next/save · esc cancel)") + "\n")
	w := 0
	for _, f := range m.fields {
		w = max(w, len(f.Label))
	}
	for i, f := range m.fields {
		prefix := "  "
		if i == m.focus && m.o == running {
			prefix = m.s.Cursor.Render("▸ ")
		}
		val := m.inputs[i].View()
		if m.o != running {
			val = strings.TrimSpace(m.inputs[i].Value())
		}
		line := prefix + fmt.Sprintf("%-*s  ", w, f.Label) + val
		if f.Hint != nil && m.o == running {
			if h := f.Hint(strings.TrimSpace(m.inputs[i].Value())); h != "" {
				line += "  " + m.s.Faint.Render(h)
			}
		}
		b.WriteString(line + "\n")
		if m.errs[i] != "" {
			b.WriteString(m.s.Error.Render("    "+m.errs[i]) + "\n")
		}
	}
	if m.o == skipped || m.o == interrupted {
		b.WriteString(m.s.Faint.Render("  cancelled") + "\n")
	}
	return tea.NewView(b.String())
}

func (m formModel) values() []string {
	out := make([]string, len(m.inputs))
	for i := range m.inputs {
		out[i] = strings.TrimSpace(m.inputs[i].Value())
	}
	return out
}

func (u *UI) formTUI(title string, fields []Field) ([]string, error) {
	fm, err := u.run(newForm(title, fields, u.S))
	if err != nil {
		return nil, err
	}
	m := fm.(formModel)
	if err := m.o.err(); err != nil {
		return nil, err
	}
	return m.values(), nil
}
