// Package testutil provides shared constants and fixtures for use across test files.
package testutil

const (
	ProfileName  = "Work"
	ProfileName2 = "Personal"

	GitName  = "Alice Example"
	GitEmail = "alice@example.com"

	GitEmail2 = "alice@personal.example.com"

	Username   = "alice"
	SSHKey     = "~/.ssh/id_example"
	SSHKey2    = "~/.ssh/id_personal"
	SigningKey  = "ABCDEF1234567890"

	SSHHostWork     = "work-server"
	SSHHostPersonal = "personal-vps"
	SSHUser         = "git"
)
