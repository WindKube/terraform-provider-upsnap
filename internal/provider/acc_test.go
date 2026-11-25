package provider

import (
	"os"
	"testing"
)

func testAccPreCheck(t *testing.T) {
	t.Helper()
	for _, env := range []string{"UPSNAP_ENDPOINT", "UPSNAP_USERNAME", "UPSNAP_PASSWORD"} {
		if os.Getenv(env) == "" {
			t.Fatalf("%s must be set for acceptance tests (run `make testacc-up`)", env)
		}
	}
}
