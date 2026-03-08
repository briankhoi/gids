package cmd

import (
	"bufio"
	"strings"
	"testing"
)

// --- prompt ---

func TestPrompt_NormalInput(t *testing.T) {
	r := bufio.NewReader(strings.NewReader("hello\n"))
	var w strings.Builder

	got, err := prompt(r, &w, "Enter: ")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "hello" {
		t.Errorf("got %q, want %q", got, "hello")
	}
}

// TestPrompt_EOFWithNoTrailingNewline verifies that a final line with no
// trailing newline is returned successfully. bufio.Reader.ReadString returns
// (data, io.EOF) in this case, which must not be treated as an error.
func TestPrompt_EOFWithNoTrailingNewline(t *testing.T) {
	r := bufio.NewReader(strings.NewReader("alice"))
	var w strings.Builder

	got, err := prompt(r, &w, "Name: ")
	if err != nil {
		t.Fatalf("got error %v, want nil (EOF with data is not an error)", err)
	}
	if got != "alice" {
		t.Errorf("got %q, want %q", got, "alice")
	}
}

func TestPrompt_TrimsWhitespace(t *testing.T) {
	r := bufio.NewReader(strings.NewReader("  alice  \n"))
	var w strings.Builder

	got, err := prompt(r, &w, "Name: ")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "alice" {
		t.Errorf("got %q, want %q", got, "alice")
	}
}

// --- promptOptional ---

func TestPromptOptional_EnterKeepsCurrent(t *testing.T) {
	r := bufio.NewReader(strings.NewReader("\n"))
	var w strings.Builder

	got, err := promptOptional(r, &w, "SSH Key", "/existing/key")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "/existing/key" {
		t.Errorf("got %q, want current value %q", got, "/existing/key")
	}
}

func TestPromptOptional_NoneClears(t *testing.T) {
	r := bufio.NewReader(strings.NewReader("none\n"))
	var w strings.Builder

	got, err := promptOptional(r, &w, "SSH Key", "/existing/key")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "" {
		t.Errorf("got %q, want empty string (cleared)", got)
	}
}

func TestPromptOptional_NewValueReplaces(t *testing.T) {
	r := bufio.NewReader(strings.NewReader("/new/key\n"))
	var w strings.Builder

	got, err := promptOptional(r, &w, "SSH Key", "/existing/key")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "/new/key" {
		t.Errorf("got %q, want %q", got, "/new/key")
	}
}

// TestPromptOptional_EOFWithNoTrailingNewline mirrors the H3 scenario for
// promptOptional — the last field in a piped input sequence.
func TestPromptOptional_EOFWithNoTrailingNewline(t *testing.T) {
	r := bufio.NewReader(strings.NewReader("/last/field"))
	var w strings.Builder

	got, err := promptOptional(r, &w, "SSH Key", "")
	if err != nil {
		t.Fatalf("got error %v, want nil", err)
	}
	if got != "/last/field" {
		t.Errorf("got %q, want %q", got, "/last/field")
	}
}

// --- promptRequired ---

func TestPromptRequired_ReturnsValue(t *testing.T) {
	r := bufio.NewReader(strings.NewReader("alice\n"))
	var w strings.Builder

	got, err := promptRequired(r, &w, "Name: ", "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "alice" {
		t.Errorf("got %q, want %q", got, "alice")
	}
}

func TestPromptRequired_UsesDefault(t *testing.T) {
	r := bufio.NewReader(strings.NewReader("\n"))
	var w strings.Builder

	got, err := promptRequired(r, &w, "Name: ", "Bob")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "Bob" {
		t.Errorf("got %q, want default %q", got, "Bob")
	}
}

func TestPromptRequired_LoopsUntilNonEmpty(t *testing.T) {
	// Two empty lines, then a valid value.
	r := bufio.NewReader(strings.NewReader("\n\nalice\n"))
	var w strings.Builder

	got, err := promptRequired(r, &w, "Name: ", "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "alice" {
		t.Errorf("got %q, want %q", got, "alice")
	}
	if !strings.Contains(w.String(), "This field is required.") {
		t.Errorf("expected re-prompt message, got: %s", w.String())
	}
}

// --- confirmPrompt ---

func TestConfirmPrompt_DefaultYes_EmptyAccepts(t *testing.T) {
	r := bufio.NewReader(strings.NewReader("\n"))
	var w strings.Builder

	got, err := confirmPrompt(r, &w, "Continue?", true)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !got {
		t.Error("expected true for empty input with defaultYes=true")
	}
}

func TestConfirmPrompt_DefaultNo_EmptyRejects(t *testing.T) {
	r := bufio.NewReader(strings.NewReader("\n"))
	var w strings.Builder

	got, err := confirmPrompt(r, &w, "Continue?", false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got {
		t.Error("expected false for empty input with defaultYes=false")
	}
}

func TestConfirmPrompt_ExplicitYes(t *testing.T) {
	for _, input := range []string{"y\n", "Y\n", "yes\n", "YES\n"} {
		r := bufio.NewReader(strings.NewReader(input))
		var w strings.Builder
		got, err := confirmPrompt(r, &w, "Continue?", false)
		if err != nil {
			t.Fatalf("input %q: unexpected error: %v", input, err)
		}
		if !got {
			t.Errorf("input %q: expected true", input)
		}
	}
}

func TestConfirmPrompt_ExplicitNo(t *testing.T) {
	for _, input := range []string{"n\n", "N\n", "no\n", "NO\n"} {
		r := bufio.NewReader(strings.NewReader(input))
		var w strings.Builder
		got, err := confirmPrompt(r, &w, "Continue?", true)
		if err != nil {
			t.Fatalf("input %q: unexpected error: %v", input, err)
		}
		if got {
			t.Errorf("input %q: expected false", input)
		}
	}
}
