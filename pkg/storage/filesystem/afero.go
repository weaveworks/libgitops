package filesystem

import (
	"context"
	"os"
	"path/filepath"

	"github.com/spf13/afero"
)

// AferoContext extends afero.Fs and afero.Afero with contexts added to every method.
type AferoContext interface {
	// RootDirectory specifies where on disk the root directory is stored.
	// This path MUST be absolute. All other paths for the other methods
	// MUST be relative to this directory.
	RootDirectory() string

	// Members of afero.Fs

	// MkdirAll creates a directory path and all parents that does not exist
	// yet.
	MkdirAll(ctx context.Context, path string, perm os.FileMode) error
	// Remove removes a file identified by name, returning an error, if any
	// happens.
	Remove(ctx context.Context, name string) error
	// Stat returns a FileInfo describing the named file, or an error, if any
	// happens.
	Stat(ctx context.Context, name string) (os.FileInfo, error)

	// Members of afero.Afero

	ReadDir(ctx context.Context, dirname string) ([]os.FileInfo, error)

	Exists(ctx context.Context, path string) (bool, error)

	ReadFile(ctx context.Context, filename string) ([]byte, error)

	WriteFile(ctx context.Context, filename string, data []byte, perm os.FileMode) error

	Walk(ctx context.Context, root string, walkFn filepath.WalkFunc) error
}

// AferoContextForLocalDir creates a new afero.OsFs for the local directory, wrapped
// in AferoContextWrapperForDir.
func AferoContextForLocalDir(rootDir string) AferoContext {
	return AferoContextWrapperForDir(afero.NewOsFs(), rootDir)
}

// AferoContextWrapperForDir wraps an underlying afero.Fs without context knowledge,
// in a AferoContext-compliant implementation; scoped at the given directory
// (i.e. wrapped in afero.NewBasePathFs(fs, rootDir)).
func AferoContextWrapperForDir(fs afero.Fs, rootDir string) AferoContext {
	// TODO: rootDir validation? It must be absolute, exist, and be a directory.
	return &aferoWithoutCtx{afero.NewBasePathFs(fs, rootDir), rootDir}
}

type aferoWithoutCtx struct {
	fs      afero.Fs
	rootDir string
}

func (a *aferoWithoutCtx) RootDirectory() string {
	return a.rootDir
}

func (a *aferoWithoutCtx) MkdirAll(_ context.Context, path string, perm os.FileMode) error {
	return a.fs.MkdirAll(path, perm)
}

func (a *aferoWithoutCtx) Remove(_ context.Context, name string) error {
	return a.fs.Remove(name)
}

func (a *aferoWithoutCtx) Stat(_ context.Context, name string) (os.FileInfo, error) {
	return a.fs.Stat(name)
}

func (a *aferoWithoutCtx) ReadDir(_ context.Context, dirname string) ([]os.FileInfo, error) {
	return afero.ReadDir(a.fs, dirname)
}

func (a *aferoWithoutCtx) Exists(_ context.Context, path string) (bool, error) {
	return afero.Exists(a.fs, path)
}

func (a *aferoWithoutCtx) ReadFile(_ context.Context, filename string) ([]byte, error) {
	return afero.ReadFile(a.fs, filename)
}

func (a *aferoWithoutCtx) WriteFile(_ context.Context, filename string, data []byte, perm os.FileMode) error {
	return afero.WriteFile(a.fs, filename, data, perm)
}

func (a *aferoWithoutCtx) Walk(_ context.Context, root string, walkFn filepath.WalkFunc) error {
	return afero.Walk(a.fs, root, walkFn)
}
