package cmd_test

import (
	"bytes"
	"strings"
	"testing"

	"gids/internal/cmd"
)

func execute(args ...string) (string, error) {
	buf := new(bytes.Buffer)
	root := cmd.NewRootCommand()
	root.SetOut(buf)
	root.SetErr(buf)
	root.SetArgs(args)
	err := root.Execute()
	return buf.String(), err
}

func TestHelp_ContainsUsage(t *testing.T) {
	out, err := execute("--help")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	for _, want := range []string{"gids", "Usage", "version", "--verbose", "profiles", "Examples"} {
		if !strings.Contains(out, want) {
			t.Errorf("help output missing %q\ngot: %s", want, out)
		}
	}
}

func TestVersion_Output(t *testing.T) {
	out, err := execute("version")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(out, "gids version") {
		t.Errorf("expected %q in output, got: %s", "gids version", out)
	}
}

func TestUnknownCommand_ReturnsError(t *testing.T) {
	_, err := execute("nonexistent")
	if err == nil {
		t.Error("expected error for unknown command, got nil")
	}
}

func TestVerboseFlag_DoesNotPanic(t *testing.T) {
	_, err := execute("--verbose", "version")
	if err != nil {
		t.Fatalf("unexpected error with --verbose: %v", err)
	}
}
