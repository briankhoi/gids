package render

import (
	"errors"
	"fmt"
	"strings"
)

// The ~/.gitconfig block. Its begin line records what gids had to change
// besides adding the block, so removing it restores the file byte-for-byte
// (SPEC §11 I3, I8) without any state kept elsewhere.
const (
	beginMarker = "# gids:begin (managed by `gids install`; remove with `gids uninstall`)"
	endMarker   = "# gids:end"
	tagNewline  = " [+newline]" // gids added a final newline the file lacked
	tagCreated  = " [created]"  // the file didn't exist before gids
)

// ErrMalformedBlock means the file has gids markers gids didn't write
// (missing end, duplicates, end before begin). gids refuses to edit such a
// file rather than guess.
var ErrMalformedBlock = errors.New("the gids block in your global git config is damaged")

// BlockBody is the text between the markers.
func BlockBody(gitconfigPath, hooksDir string) string {
	return "[include]\n\tpath = " + Quote(gitconfigPath) + "\n[core]\n\thooksPath = " + Quote(hooksDir) + "\n"
}

type span struct {
	start, end int // byte offsets; [start,end) covers the block lines incl. final "\n"
	beginLine  string
}

// findBlock locates the single well-formed block, if any.
func findBlock(content string) (span, bool, error) {
	var s span
	found, open := false, false
	off := 0
	for off < len(content) {
		nl := strings.IndexByte(content[off:], '\n')
		lineEnd := len(content)
		if nl >= 0 {
			lineEnd = off + nl + 1
		}
		line := strings.TrimRight(content[off:lineEnd], "\r\n")
		switch {
		case strings.HasPrefix(line, "# gids:begin"):
			if open || found || !isBeginLine(line) {
				return span{}, false, fmt.Errorf("%w: unexpected %q", ErrMalformedBlock, line)
			}
			open, s.start, s.beginLine = true, off, line
		case strings.HasPrefix(line, "# gids:end"):
			if !open || line != endMarker {
				return span{}, false, fmt.Errorf("%w: unexpected %q", ErrMalformedBlock, line)
			}
			open, found, s.end = false, true, lineEnd
		}
		off = lineEnd
	}
	if open {
		return span{}, false, fmt.Errorf("%w: %q has no matching %q", ErrMalformedBlock, beginMarker, endMarker)
	}
	return s, found, nil
}

func isBeginLine(l string) bool {
	return l == beginMarker || l == beginMarker+tagNewline || l == beginMarker+tagCreated
}

// HasBlock reports whether content contains a well-formed gids block.
func HasBlock(content string) (bool, error) {
	_, ok, err := findBlock(content)
	return ok, err
}

// WithBlock returns content with the gids block (containing body) at the end.
// An existing block is replaced in place only if it's already last; otherwise
// it's moved to the end so it still comes after the user's [user] settings.
// exists says whether the file existed at all.
func WithBlock(content, body string, exists bool) (string, error) {
	s, found, err := findBlock(content)
	if err != nil {
		return "", err
	}
	tag := ""
	if found {
		tag = strings.TrimPrefix(s.beginLine, beginMarker)
		if s.end != len(content) && tag == tagNewline {
			// Moving the block: the newline gids added is no longer at the
			// end, so it can't be told apart from the user's. Keep it.
			tag = ""
		}
		content = content[:s.start] + content[s.end:]
	} else if !exists {
		tag = tagCreated
	} else if content != "" && !strings.HasSuffix(content, "\n") {
		tag = tagNewline
	}
	if tag == tagNewline && !strings.HasSuffix(content, "\n") {
		content += "\n"
	}
	return content + beginMarker + tag + "\n" + body + endMarker + "\n", nil
}

// WithoutBlock removes the gids block, undoing exactly what WithBlock added.
// remove reports that the file should be deleted (gids created it and nothing
// else was added since).
func WithoutBlock(content string) (out string, remove bool, err error) {
	s, found, err := findBlock(content)
	if err != nil || !found {
		return content, false, err
	}
	tag := strings.TrimPrefix(s.beginLine, beginMarker)
	out = content[:s.start] + content[s.end:]
	if tag == tagNewline && s.end == len(content) && strings.HasSuffix(out, "\n") {
		out = out[:len(out)-1]
	}
	if tag == tagCreated && out == "" {
		return "", true, nil
	}
	return out, false, nil
}
