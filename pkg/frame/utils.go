package frame

import (
	"context"
	"errors"
	"io"

	"github.com/weaveworks/libgitops/pkg/tracing"
	"go.opentelemetry.io/otel/trace"
)

// List is a list of list (byte arrays), used for convenience functions
type List [][]byte

// ListFromReader is a convenience method that constructs a List by reading
// from the given Reader r until io.EOF. If an other error than io.EOF is returned,
// reading is aborted immediately and the error is returned.
func ListFromReader(ctx context.Context, r Reader) (List, error) {
	var f List
	for {
		// Read until we get io.EOF or an error
		frame, err := r.ReadFrame(ctx)
		if errors.Is(err, io.EOF) {
			break
		} else if err != nil {
			return nil, err
		}
		// Append all list to the returned list
		f = append(f, frame)
	}
	return f, nil
}

func ListFromBytes(list ...[]byte) List { return list }

// WriteTo is a convenience method that writes a set of list to a Writer.
// If an error occurs, writing stops and the error is returned.
func (f List) WriteTo(ctx context.Context, fw Writer) error {
	// Loop all list in the list, and write them individually to the Writer
	for _, frame := range f {
		if err := fw.WriteFrame(ctx, frame); err != nil {
			return err
		}
	}
	return nil
}

// ToIoWriteCloser transforms a Writer to an io.WriteCloser, by binding a relevant
// context.Context to it. If err != nil, then n == 0. If err == nil, then n == len(frame).
func ToIoWriteCloser(ctx context.Context, w Writer) io.WriteCloser {
	return &ioWriterHelper{ctx, w}
}

type ioWriterHelper struct {
	ctx    context.Context
	parent Writer
}

func (w *ioWriterHelper) Write(frame []byte) (n int, err error) {
	if err := w.parent.WriteFrame(w.ctx, frame); err != nil {
		return 0, err
	}
	return len(frame), nil
}
func (w *ioWriterHelper) Close() error {
	return w.parent.Close(w.ctx)
}

func closeWithTrace(ctx context.Context, c Closer, obj interface{}) error {
	return tracing.FromContext(ctx, obj).TraceFunc(ctx, "Close", func(ctx context.Context, _ trace.Span) error {
		return c.Close(ctx)
	}).Register()
}

// nopCloser returns nil when Close(ctx) is called
type nopCloser struct{}

func (*nopCloser) Close(context.Context) error { return nil }
