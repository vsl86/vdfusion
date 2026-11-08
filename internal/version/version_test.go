package version

import (
	"os"
	"testing"
)

func TestEnvOverride(t *testing.T) {
	// Save and restore original values
	orig := Version
	origCommit := Commit
	t.Cleanup(func() {
		Version = orig
		Commit = origCommit
		os.Unsetenv("APP_VERSION")
		os.Unsetenv("APP_COMMIT")
	})

	os.Setenv("APP_VERSION", "v9.9.9")
	os.Setenv("APP_COMMIT", "testsha")

	// Re-invoke the override logic (init already ran, so call directly)
	applyEnvOverrides()

	if Version != "v9.9.9" {
		t.Errorf("expected Version=v9.9.9, got %s", Version)
	}
	if Commit != "testsha" {
		t.Errorf("expected Commit=testsha, got %s", Commit)
	}
}
