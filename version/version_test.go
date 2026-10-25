package version

import (
	"os"
	"testing"
)

func TestVersionBuildOverride(t *testing.T) {
	expected := os.Getenv("INGRESS_TEST_VERSION")
	if expected == "" {
		t.Skip("set INGRESS_TEST_VERSION and matching -ldflags -X to verify release version injection")
	}
	if Version != expected {
		t.Fatalf("version = %q, want injected release version %q", Version, expected)
	}
}
