package stream

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing/iotest"

	"github.com/weaveworks/libgitops/pkg/stream/metadata"
)

// newErrReader makes a Reader implementation that only returns the given error on Read()
func newErrReader(err error, opts ...metadata.HeaderOption) Reader {
	return NewReader(iotest.ErrReader(err), opts...)
}

const (
	stdinPath  = "/dev/stdin"
	stdoutPath = "/dev/stdout"
	stderrPath = "/dev/stderr"
)

func FromStdin(opts ...metadata.HeaderOption) Reader {
	return FromFile(stdinPath, opts...)
}

// FromFile returns an io.ReadCloser from the given file, or an io.ReadCloser which returns
// the given file open error when read.
func FromFile(filePath string, opts ...metadata.HeaderOption) Reader {
	// Support stdin
	if filePath == "-" || filePath == stdinPath {
		// Mark the source as /dev/stdin
		opts = append(opts, metadata.WithContentLocation(stdinPath))
		// TODO: Maybe have a way to override the TracerName through Metadata?
		return NewReader(os.Stdin, opts...)
	}

	// Report the file path in the X-Content-Location header
	opts = append(opts, metadata.WithContentLocation(filePath))

	// Open the file
	f, err := os.Open(filePath)
	if err != nil {
		return newErrReader(err, opts...)
	}
	fi, err := f.Stat()
	if err != nil {
		return newErrReader(err, opts...)
	}

	// Register the Content-Length header
	opts = append(opts, metadata.WithContentLength(fi.Size()))

	return NewReader(f, opts...)
}

// FromBytes returns an io.Reader from the given byte stream.
func FromBytes(content []byte, opts ...metadata.HeaderOption) Reader {
	// Register the Content-Length
	opts = append(opts, metadata.WithContentLength(int64(len(content))))
	// Read from a *bytes.Reader
	return NewReader(bytes.NewReader(content), opts...)
}

// FromString returns an io.Reader from the given string stream.
func FromString(content string, opts ...metadata.HeaderOption) Reader {
	// Register the Content-Length
	opts = append(opts, metadata.WithContentLength(int64(len(content))))
	// Read from a *strings.Reader
	return NewReader(strings.NewReader(content), opts...)
}

// TODO: FromHTTPResponse and ToHTTPResponse

func ToStdout(opts ...metadata.HeaderOption) Writer {
	return ToFile(stdoutPath, opts...)
}
func ToStderr(opts ...metadata.HeaderOption) Writer {
	return ToFile(stderrPath, opts...)
}
func ToBuffer(buf *bytes.Buffer, opts ...metadata.HeaderOption) Writer {
	return NewWriter(buf, opts...)
}

func ToFile(filePath string, opts ...metadata.HeaderOption) Writer {
	// Shorthands for pipe IO
	if filePath == "-" || filePath == stdoutPath {
		// Mark the target as /dev/stdout
		opts = append(opts, metadata.WithContentLocation(stdoutPath))
		return NewWriter(os.Stdout, opts...)
	}
	if filePath == stderrPath {
		// Mark the target as /dev/stderr
		opts = append(opts, metadata.WithContentLocation(stderrPath))
		return NewWriter(os.Stderr, opts...)
	}

	// Report the file path in the X-Content-Location header
	opts = append(opts, metadata.WithContentLocation(filePath))

	// Make sure all directories are created
	if err := os.MkdirAll(filepath.Dir(filePath), 0755); err != nil {
		return newErrWriter(err, opts...)
	}

	// Create or truncate the file
	f, err := os.Create(filePath)
	if err != nil {
		return newErrWriter(err, opts...)
	}

	// Register the Content-Length header
	fi, err := f.Stat()
	if err != nil {
		return newErrWriter(err, opts...)
	}
	opts = append(opts, metadata.WithContentLength(fi.Size()))

	return NewWriter(f, opts...)
}

func newErrWriter(err error, opts ...metadata.HeaderOption) Writer {
	return NewWriter(&errWriter{err}, opts...)
}

type errWriter struct{ err error }

func (w *errWriter) Write([]byte) (int, error) { return 0, w.err }
