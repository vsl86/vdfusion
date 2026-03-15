package version

var (
	// Version is the current version of the application.
	// This is intended to be overridden at build time using ldflags:
	// -ldflags "-X vdfusion/internal/version.Version=v1.0.0"
	Version = "v0.0.0-dev"

	// Commit is the git commit hash the application was built from.
	// This is intended to be overridden at build time using ldflags:
	// -ldflags "-X vdfusion/internal/version.Commit=abcd123"
	Commit = "none"
)
