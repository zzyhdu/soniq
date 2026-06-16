package version

import (
	"strings"
	"testing"
)

func TestSummaryIncludesServiceAndBuildMetadata(t *testing.T) {
	output := Summary("soniq-api")

	for _, want := range []string{"soniq-api", "version=", "commit=", "build_date="} {
		if !strings.Contains(output, want) {
			t.Fatalf("Summary() = %q, want it to contain %q", output, want)
		}
	}
}
