package filesystem

import (
	"context"
	"io/fs"
)

// ListValidFilesInFilesystem discovers files in the given Filesystem that has a
// ContentType that contentTyper recognizes, and is not a path that is excluded by
// pathExcluder.
func ListValidFilesInFilesystem(ctx context.Context, givenFs Filesystem, contentTyper ContentTyper, pathExcluder PathExcluder) (files []string, err error) {
	fsys := givenFs.WithContext(ctx)
	err = fs.WalkDir(fsys, "", func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}

		// Only include valid files
		if !d.IsDir() && IsValidFileInFilesystem(ctx, givenFs, contentTyper, pathExcluder, path) {
			files = append(files, path)
		}
		return nil
	})
	return
}

// IsValidFileInFilesystem checks if file (a relative path) has a ContentType
// that contentTyper recognizes, and is not a path that is excluded by pathExcluder.
func IsValidFileInFilesystem(ctx context.Context, fs Filesystem, contentTyper ContentTyper, pathExcluder PathExcluder, file string) bool {
	// return false if this path should be excluded
	// pathExcluder can be nil; watch out for that
	if pathExcluder != nil && pathExcluder.ShouldExcludePath(file) {
		return false
	}

	// If the content type is valid for this path, err == nil => return true
	_, err := contentTyper.ContentTypeForPath(ctx, fs, file)
	return err == nil
}
