package version

import "os"

var (
	// Version is the current version of the application.
	// This is intended to be overridden at build time using ldflags:
	//   -ldflags "-X vdfusion/internal/version.Version=v1.0.0"
	// At runtime it can also be overridden via the APP_VERSION env var,
	// which is useful when a pre-built Docker image is promoted to a new
	// release tag without a full rebuild.
	Version = "v0.0.0-dev"

	// Commit is the git commit hash the application was built from.
	// This is intended to be overridden at build time using ldflags:
	//   -ldflags "-X vdfusion/internal/version.Commit=abcd123"
	Commit = "none"
)

func init() {
	applyEnvOverrides()
}

// applyEnvOverrides reads APP_VERSION / APP_COMMIT env vars and
// updates the package-level vars if set. Called from init() and
// exposed for testing.
func applyEnvOverrides() {
	if v := os.Getenv("APP_VERSION"); v != "" {
		Version = v
	}
	if c := os.Getenv("APP_COMMIT"); c != "" {
		Commit = c
	}
}
