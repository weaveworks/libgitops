package frame

import (
	"context"
	"io"

	"github.com/weaveworks/libgitops/pkg/stream"
)

func newDelegatingWriter(ct stream.ContentType, w stream.Writer) Writer {
	return &delegatingWriter{
		// TODO: Register options?
		MetadataContainer: w.ContentMetadata().Clone().ToContainer(),
		ContentTyped:      ct,
		w:                 w,
	}
}

// delegatingWriter is an implementation of the Writer interface
type delegatingWriter struct {
	stream.MetadataContainer
	stream.ContentTyped
	w stream.Writer
}

func (w *delegatingWriter) WriteFrame(ctx context.Context, frame []byte) error {
	// Write the frame to the underlying writer
	n, err := w.w.WithContext(ctx).Write(frame)
	// Guard against short writes
	return catchShortWrite(n, err, frame)
}

func (w *delegatingWriter) Close(ctx context.Context) error { return w.w.WithContext(ctx).Close() }

func newErrWriter(ct stream.ContentType, err error, meta stream.Metadata) Writer {
	return &errWriter{
		meta.Clone().ToContainer(),
		ct,
		&nopCloser{},
		err,
	}
}

type errWriter struct {
	stream.MetadataContainer
	stream.ContentTyped
	Closer
	err error
}

func (w *errWriter) WriteFrame(context.Context, []byte) error { return w.err }

func catchShortWrite(n int, err error, frame []byte) error {
	if n < len(frame) && err == nil {
		err = io.ErrShortWrite
	}
	return err
}
