package watcher

import (
	"os"
	"path/filepath"
	"strings"
)

func (w *FileWatcher) getFiles() ([]string, error) {
	return WalkDirectoryForFiles(w.dir, w.opts.ValidExtensions, w.opts.ExcludeDirs)
}

func (w *FileWatcher) validFile(path string) bool {
	return isValidFile(path, w.opts.ValidExtensions, w.opts.ExcludeDirs)
}

// WalkDirectoryForFiles discovers all subdirectories and
// returns a list of valid files in them
func WalkDirectoryForFiles(dir string, validExts, excludeDirs []string) (files []string, err error) {
	err = filepath.Walk(dir,
		func(path string, info os.FileInfo, err error) error {
			if err != nil {
				return err
			}

			if !info.IsDir() {
				// Only include valid files
				if isValidFile(path, validExts, excludeDirs) {
					files = append(files, path)
				}
			}

			return nil
		})

	return
}

// isValidFile is used to filter out all unsupported
// files based on if their extension is unknown or
// if their path contains an excluded directory
func isValidFile(path string, validExts, excludeDirs []string) bool {
	parts := strings.Split(filepath.Clean(path), string(os.PathSeparator))
	ext := filepath.Ext(parts[len(parts)-1])
	for _, suffix := range validExts {
		if ext == suffix {
			return true
		}
	}

	for i := 0; i < len(parts)-1; i++ {
		for _, exclude := range excludeDirs {
			if parts[i] == exclude {
				return false
			}
		}
	}

	return false
}
