package version

// Version is the current version of gids.
// Overridden at build time via: -ldflags "-X gids/internal/version.Version=x.y.z"
var Version = "dev"
