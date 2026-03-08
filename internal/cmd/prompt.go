package cmd

import (
	"bufio"
	"fmt"
	"io"
	"strings"
)

// confirmPrompt writes "<message> [Y/n]: " (or [y/N]) to w and reads a yes/no
// answer. defaultYes controls the default on empty input and which letter is
// uppercased. Accepts y/yes (true) or n/no (false), case-insensitive. Re-prompts
// on unrecognized input.
func confirmPrompt(r *bufio.Reader, w io.Writer, message string, defaultYes bool) (bool, error) {
	opts := "[y/N]"
	if defaultYes {
		opts = "[Y/n]"
	}
	for {
		val, err := prompt(r, w, fmt.Sprintf("%s %s: ", message, opts))
		if err != nil {
			return false, err
		}
		switch strings.ToLower(val) {
		case "":
			return defaultYes, nil
		case "y", "yes":
			return true, nil
		case "n", "no":
			return false, nil
		default:
			fmt.Fprintln(w, "Please enter y or n.")
		}
	}
}

// prompt writes message to w and reads a trimmed line from r.
// io.EOF with data (final line without trailing newline) is treated as success.
func prompt(r *bufio.Reader, w io.Writer, message string) (string, error) {
	fmt.Fprint(w, message)
	line, err := r.ReadString('\n')
	if err != nil && err != io.EOF {
		return "", err
	}
	return strings.TrimSpace(line), nil
}

// promptOptional prompts for an optional field with keep/clear semantics:
// empty input keeps current; "none" clears it.
func promptOptional(r *bufio.Reader, w io.Writer, label, current string) (string, error) {
	val, err := prompt(r, w, fmt.Sprintf("%s [%s] (Enter to keep, \"none\" to clear): ", label, current))
	if err != nil {
		return "", err
	}
	switch val {
	case "none":
		return "", nil
	case "":
		return current, nil
	default:
		return val, nil
	}
}

// promptRequired prompts repeatedly until a non-empty value is entered.
// If defaultVal is non-empty, an empty input returns defaultVal.
func promptRequired(r *bufio.Reader, w io.Writer, message, defaultVal string) (string, error) {
	for {
		val, err := prompt(r, w, message)
		if err != nil {
			return "", err
		}
		if val == "" && defaultVal != "" {
			return defaultVal, nil
		}
		if val != "" {
			return val, nil
		}
		fmt.Fprintln(w, "This field is required.")
	}
}
