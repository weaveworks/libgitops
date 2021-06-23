package git

import (
	"context"
	"os"
	"path/filepath"

	"github.com/go-git/go-git/v5"
	"github.com/spf13/afero"
)

type Filesystem struct {
	git *goGit
}

func (f *Filesystem) RootDirectory() string {
	return f.rootDir
}

func (f *Filesystem) Checksum(_ context.Context, filename string) (string, error) {
	// Get the latest commit that is touching this file
	ci, err := f.git.repo.Log(&git.LogOptions{
		Order:    git.LogOrderCommitterTime,
		FileName: &filename,
	})
	if err != nil {
		return "", err
	}
	commit, err := ci.Next()
	if err != nil {
		return "", err
	}
	return commit.Hash.String(), nil
}

func (f *Filesystem) MkdirAll(_ context.Context, path string, perm os.FileMode) error {
	return f.fs.MkdirAll(path, perm)
}

func (f *Filesystem) Remove(_ context.Context, name string) error {
	return f.fs.Remove(name)
}

func (f *Filesystem) Stat(_ context.Context, name string) (os.FileInfo, error) {
	return f.fs.Stat(name)
}

func (f *Filesystem) ReadDir(_ context.Context, dirname string) ([]os.FileInfo, error) {
	return afero.ReadDir(f.fs, dirname)
}

func (f *Filesystem) Exists(_ context.Context, path string) (bool, error) {
	return afero.Exists(f.fs, path)
}

func (f *Filesystem) ReadFile(_ context.Context, filename string) ([]byte, error) {
	return afero.ReadFile(f.fs, filename)
}

func (f *Filesystem) WriteFile(_ context.Context, filename string, data []byte, perm os.FileMode) error {
	return afero.WriteFile(f.fs, filename, data, perm)
}

func (f *Filesystem) Walk(_ context.Context, root string, walkFn filepath.WalkFunc) error {
	return afero.Walk(f.fs, root, walkFn)
}
