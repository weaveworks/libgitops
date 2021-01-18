package core

import (
	"context"
	"os"
	"path/filepath"

	"github.com/spf13/afero"
)

// AferoContext extends afero.Fs and afero.Afero with contexts added to every method.
type AferoContext interface {
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

// AferoWithoutContext wraps an underlying afero.Fs without context knowledge,
// in a AferoContext-compliant implementation.
func AferoWithoutContext(fs afero.Fs) AferoContext {
	return &aferoWithoutCtx{fs}
}

type aferoWithoutCtx struct {
	fs afero.Fs
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
