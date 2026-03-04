package config

// Profile represents a single Git identity.
type Profile struct {
	Name       string `yaml:"name"`                  // display name, e.g. "Work"
	GitName    string `yaml:"git_name"`              // user.name
	GitEmail   string `yaml:"git_email"`             // user.email
	Username   string `yaml:"username,omitempty"`    // Git username - sets credential.username (HTTPS disambiguation, no secret stored)
	SSHKey     string `yaml:"ssh_key,omitempty"`     // path to SSH key - sets core.sshCommand
	SigningKey string `yaml:"signing_key,omitempty"` // GPG key fingerprint or SSH key path for commit signing
}

// gids stores zero secret material — only references, paths, and usernames.

// AppConfig is the top-level structure persisted to config.yaml.
type AppConfig struct {
	Profiles []Profile         `yaml:"profiles"`
	Rules    map[string]string `yaml:"rules,omitempty"` // glob -> profile name
}
