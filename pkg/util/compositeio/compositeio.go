package compositeio

import (
	"fmt"
	"io"

	"github.com/weaveworks/libgitops/pkg/tracing"
)

func ReadCloser(r io.Reader, c io.Closer) io.ReadCloser {
	return readCloser{r, c}
}

type readCloser struct {
	io.Reader
	io.Closer
}

func (rc readCloser) TracerName() string {
	return fmt.Sprintf("compositeio.readCloser{%T, %T}", rc.Reader, rc.Closer)
}

var _ tracing.TracerNamed = readCloser{}

func WriteCloser(w io.Writer, c io.Closer) io.WriteCloser {
	return writeCloser{w, c}
}

type writeCloser struct {
	io.Writer
	io.Closer
}

func (wc writeCloser) TracerName() string {
	return fmt.Sprintf("compositeio.writeCloser{%T, %T}", wc.Writer, wc.Closer)
}

var _ tracing.TracerNamed = writeCloser{}
