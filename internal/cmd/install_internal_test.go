package cmd

import "testing"

func TestTransientExe(t *testing.T) {
	for exe, want := range map[string]bool{
		"/var/folders/x/T/go-build123/b001/exe/gids": true,
		"/tmp/go-build9/b001/exe/gids":               true,
		"/Users/b/go/bin/gids":                       false,
		"/usr/local/bin/gids":                        false,
		"/tmp/gids-bin-1/gids":                       false,
	} {
		if got := transientExe(exe); got != want {
			t.Errorf("transientExe(%q) = %v, want %v", exe, got, want)
		}
	}
}
