package version

// version is set at build time via: -ldflags "-X gids/internal/version.version=x.y.z"
var version = "dev"

// Get returns the current version of gids.
func Get() string { return version }
