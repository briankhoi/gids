package git

import (
	"fmt"
	"strings"
)

// Ident is a commit author or committer.
type Ident struct {
	Name, Email string
	Date        string // git internal format: "<unix seconds> <+hhmm>"
}

func (i Ident) String() string { return i.Name + " <" + i.Email + ">" }

// Commit is a raw commit object.
type Commit struct {
	SHA       string
	Tree      string
	Parents   []string
	Author    Ident
	Committer Ident
	Encoding  string   // "" means UTF-8
	Dropped   []string // headers a rewrite can't keep (gpgsig, mergetag…)
	Message   string   // exact bytes after the header block
}

// ReadCommit parses `git cat-file commit sha`.
func (c Client) ReadCommit(sha string) (Commit, error) {
	full, err := c.Run("rev-parse", "--verify", "--end-of-options", sha+"^{commit}")
	if err != nil {
		return Commit{}, err
	}
	raw, err := c.exec("", "cat-file", "commit", full)
	if err != nil {
		return Commit{}, err
	}
	head, msg, ok := strings.Cut(raw, "\n\n")
	if !ok { // a commit with no message has no blank separator line
		head, msg = strings.TrimSuffix(raw, "\n"), ""
	}
	cm := Commit{SHA: full, Message: msg}
	for line := range strings.SplitSeq(head, "\n") {
		if strings.HasPrefix(line, " ") { // continuation of a multi-line header
			continue
		}
		key, val, _ := strings.Cut(line, " ")
		switch key {
		case "tree":
			cm.Tree = val
		case "parent":
			cm.Parents = append(cm.Parents, val)
		case "author":
			if cm.Author, err = parseIdent(val); err != nil {
				return Commit{}, fmt.Errorf("commit %s: %w", full, err)
			}
		case "committer":
			if cm.Committer, err = parseIdent(val); err != nil {
				return Commit{}, fmt.Errorf("commit %s: %w", full, err)
			}
		case "encoding":
			cm.Encoding = val
		default:
			cm.Dropped = append(cm.Dropped, key)
		}
	}
	if cm.Tree == "" {
		return Commit{}, fmt.Errorf("commit %s has no tree", full)
	}
	return cm, nil
}

// parseIdent parses "Name <email> 1700000000 -0800".
func parseIdent(s string) (Ident, error) {
	lt := strings.IndexByte(s, '<')
	gt := strings.LastIndexByte(s, '>')
	if lt < 0 || gt < lt {
		return Ident{}, fmt.Errorf("malformed ident %q", s)
	}
	return Ident{
		Name:  strings.TrimSpace(s[:lt]),
		Email: s[lt+1 : gt],
		Date:  strings.TrimSpace(s[gt+1:]),
	}, nil
}

// CommitTree writes a commit object and returns its sha. The message is
// stored byte-for-byte; dates are kept exactly as given.
func (c Client) CommitTree(tree string, parents []string, author, committer Ident, encoding, msg string) (string, error) {
	args := []string{}
	if encoding != "" {
		args = append(args, "-c", "i18n.commitEncoding="+encoding)
	}
	args = append(args, "commit-tree", tree)
	for _, p := range parents {
		args = append(args, "-p", p)
	}
	args = append(args, "-F", "-")
	cl := c
	cl.Env = append(append([]string{}, c.Env...),
		"GIT_AUTHOR_NAME="+author.Name, "GIT_AUTHOR_EMAIL="+author.Email, "GIT_AUTHOR_DATE="+author.Date,
		"GIT_COMMITTER_NAME="+committer.Name, "GIT_COMMITTER_EMAIL="+committer.Email, "GIT_COMMITTER_DATE="+committer.Date,
	)
	return cl.RunInput(msg, args...)
}
