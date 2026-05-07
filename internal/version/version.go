package version

import "fmt"

var (
	Version   = "dev"
	Commit    = "unknown"
	Branch    = "unknown"
	BuildTime = "unknown"
	BuildBy   = "unknown"
)

func String() string {
	return fmt.Sprintf("%s (%s)", Version, Commit)
}
