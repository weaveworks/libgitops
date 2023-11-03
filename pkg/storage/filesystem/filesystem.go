package filesystem

import (
	"context"
	"errors"
	"io/fs"
	"os"
	"strconv"

	"github.com/spf13/afero"
	"github.com/weaveworks/libgitops/pkg/storage/commit"
)

type Filesystem interface {
	WithContext(ctx context.Context) FS
	RefResolver() commit.RefResolver
}

type FS interface {
	fs.StatFS
	fs.ReadDirFS
	fs.ReadFileFS

	// MkdirAll creates a directory path and all parents that does not exist
	// yet.
	MkdirAll(path string, perm os.FileMode) error
	// Remove removes a file identified by name, returning an error, if any
	// happens.
	Remove(name string) error

	WriteFile(filename string, data []byte, perm os.FileMode) error

	// Custom methods

	// Checksum returns a checksum of the given file.
	//
	// What the checksum is is application-dependent, however, it
	// should be the same for two invocations, as long as the stored
	// data is the same. It might change over time although the
	// underlying data did not. Examples of checksums that can be
	// used is: the file modification timestamp, a sha256sum of the
	// file content, or the latest Git commit when the file was
	// changed.
	//
	// Like Stat(filename), os.ErrNotExist is returned if the file does
	// not exist, such that errors.Is(err, os.ErrNotExist) can be used
	// to check.
	Checksum(filename string) (string, error)

	// RootDirectory specifies where on disk the root directory is stored.
	// This path MUST be absolute. All other paths for the other methods
	// MUST be relative to this directory.
	//RootDirectory() (string, error)
}

type ContextFS interface {
	Open(ctx context.Context, name string) (fs.File, error)
	Stat(ctx context.Context, name string) (fs.FileInfo, error)
	ReadDir(ctx context.Context, name string) ([]fs.DirEntry, error)
	ReadFile(ctx context.Context, name string) ([]byte, error)
	MkdirAll(ctx context.Context, path string, perm os.FileMode) error
	Remove(ctx context.Context, name string) error
	WriteFile(ctx context.Context, filename string, data []byte, perm os.FileMode) error
	Checksum(ctx context.Context, filename string) (string, error)
	//RootDirectory(ctx context.Context) (string, error)
}

// Exists uses the ctxFs.Stat() method to check whether the file exists.
// If os.ErrNotExist is returned from the stat call, the return value is
// false, nil. If another error occurred, then false, err is returned.
// If err == nil, then true, nil is returned.
func Exists(ctxFs FS, name string) (bool, error) {
	_, err := ctxFs.Stat(name)
	if errors.Is(err, os.ErrNotExist) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return true, nil
}

func FromContext(ctxFs ContextFS) Filesystem {
	return &fromCtxFs{ctxFs}
}

type fromCtxFs struct {
	ctxFs ContextFS
}

func (f *fromCtxFs) WithContext(ctx context.Context) FS {
	return &fromCtxFsMapper{f, ctx}
}

type fromCtxFsMapper struct {
	*fromCtxFs
	ctx context.Context
}

func (f *fromCtxFsMapper) Open(name string) (fs.File, error) {
	return f.ctxFs.Open(f.ctx, name)
}
func (f *fromCtxFsMapper) Stat(name string) (fs.FileInfo, error) {
	return f.ctxFs.Stat(f.ctx, name)
}
func (f *fromCtxFsMapper) ReadDir(name string) ([]fs.DirEntry, error) {
	return f.ctxFs.ReadDir(f.ctx, name)
}
func (f *fromCtxFsMapper) ReadFile(name string) ([]byte, error) {
	return f.ctxFs.ReadFile(f.ctx, name)
}
func (f *fromCtxFsMapper) MkdirAll(path string, perm os.FileMode) error {
	return f.ctxFs.MkdirAll(f.ctx, path, perm)
}
func (f *fromCtxFsMapper) Remove(name string) error {
	return f.ctxFs.Remove(f.ctx, name)
}
func (f *fromCtxFsMapper) WriteFile(filename string, data []byte, perm os.FileMode) error {
	return f.ctxFs.WriteFile(f.ctx, filename, data, perm)
}
func (f *fromCtxFsMapper) Checksum(filename string) (string, error) {
	return f.ctxFs.Checksum(f.ctx, filename)
}

// NewOSFilesystem creates a new afero.OsFs for the local directory, using
// NewFilesystem underneath.
func NewOSFilesystem(rootDir string) Filesystem {
	return FilesystemFromAfero(afero.NewOsFs())
}

// NewFilesystem wraps an underlying afero.Fs without context knowledge,
// in a Filesystem-compliant implementation; scoped at the given directory
// (i.e. wrapped in afero.NewBasePathFs(fs, rootDir)).
//
// Checksum is calculated based on the modification timestamp of the file.
func FilesystemFromAfero(fs afero.Fs) Filesystem {
	// TODO: rootDir validation? It must be absolute, exist, and be a directory.
	return &nopCtx{&filesystem{afero.NewIOFS(fs)}}
}

type nopCtx struct {
	fs FS
}

func (c *nopCtx) WithContext(context.Context) FS { return c.fs }

type filesystem struct {
	afero.IOFS
}

func (f *filesystem) WriteFile(filename string, data []byte, perm os.FileMode) error {
	return afero.WriteFile(f.IOFS.Fs, filename, data, perm)
}
func (f *filesystem) Checksum(filename string) (string, error) {
	fi, err := f.Stat(filename)
	if err != nil {
		return "", err
	}
	return checksumFromFileInfo(fi), nil
}

func checksumFromFileInfo(fi os.FileInfo) string {
	return strconv.FormatInt(fi.ModTime().UnixNano(), 10)
}
