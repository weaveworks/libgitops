package filesystem

import (
	"context"
	"path/filepath"
)

// PathExcluder is an interface that lets the user implement custom policies
// for whether a given relative path to a given directory (fs is scoped at
// that directory) should be considered for an operation (e.g. inotify watch
// or file search).
type PathExcluder interface {
	// ShouldExcludePath takes in a context, the fs filesystem abstraction,
	// and a relative path to the file which should be determined if it should
	// be excluded or not.
	ShouldExcludePath(ctx context.Context, fs AferoContext, path string) bool
}

// ExcludeGitDirectory implements PathExcluder.
var _ PathExcluder = ExcludeGitDirectory{}

// ExcludeGitDirectory is a sample implementation of PathExcluder, that excludes
// all files under a ".git" directory, anywhere in the tree under the root directory.
type ExcludeGitDirectory struct{}

func (ExcludeGitDirectory) ShouldExcludePath(_ context.Context, _ AferoContext, path string) bool {
	// Always start from a clean path
	path = filepath.Clean(path)
	for {
		// get the current base entry name
		baseName := filepath.Base(path)
		// This means path is now an empty string; we did not find a .git directory anywhere
		if baseName == "." {
			return false
		}
		// We possibly found a directory named git; this path should be excluded
		if baseName == ".git" {
			return true
		}
		// "go up" one directory for the next iteration
		path = filepath.Dir(path)
	}
}
