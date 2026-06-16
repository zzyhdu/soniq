package version

import "fmt"

// These values are overridden by release builds through Go ldflags.
var (
	Version   = "dev"
	Commit    = "unknown"
	BuildDate = "unknown"
)

// Summary formats build metadata for CLIs and diagnostics.
func Summary(service string) string {
	if service == "" {
		service = "soniq"
	}
	return fmt.Sprintf("%s version=%s commit=%s build_date=%s", service, Version, Commit, BuildDate)
}
