package testenv

import "os/exec"

// ownGroup is a no-op on Windows (CommandContext kills the direct child).
func ownGroup(*exec.Cmd) {}
