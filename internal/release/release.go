// Package release holds build metadata set by ko through -ldflags.
package release

// Set at link time; "dev" for go run and tests.
var (
	version    = "dev"
	commit     = ""
	commitTime = ""
)

// Version is the release version, "dev" outside a release build.
func Version() string { return version }

// Commit is the source commit, "" outside a release build.
func Commit() string { return commit }

// CommitTime is the commit timestamp, "" outside a release build.
func CommitTime() string { return commitTime }
