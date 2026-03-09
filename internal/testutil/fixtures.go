// Package testutil provides shared constants and fixtures for use across test files.
package testutil

const (
	ProfileName  = "Work"
	ProfileName2 = "Personal"
	ProfileName3 = "OSS"

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

	// Rule engine test fixtures: a synthetic home directory and associated
	// glob patterns / paths used across rules and rule-command tests.
	RuleHome              = "/home/user"
	RuleGlobWork          = "/home/user/work/*"
	RuleGlobWorkTilde     = "~/work/*"
	RuleGlobOSS           = "/home/user/work/oss/*"
	RuleGlobPersonalTilde = "~/personal/*"
	RulePathExact         = "/home/user/work"
	RulePathWork          = "/home/user/work/myproject"
	RulePathPersonal      = "/home/user/personal/project"
	RulePathOSS           = "/home/user/work/oss/myproject"
)
