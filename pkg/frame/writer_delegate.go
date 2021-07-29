package frame

import (
	"context"
	"io"

	"github.com/weaveworks/libgitops/pkg/content"
)

func newDelegatingWriter(ct content.ContentType, w content.Writer) Writer {
	return &delegatingWriter{
		// TODO: Register options?
		MetadataContainer: w.ContentMetadata().Clone().ToContainer(),
		ContentTyped:      ct,
		w:                 w,
	}
}

// delegatingWriter is an implementation of the Writer interface
type delegatingWriter struct {
	content.MetadataContainer
	content.ContentTyped
	w content.Writer
}

func (w *delegatingWriter) WriteFrame(ctx context.Context, frame []byte) error {
	// Write the frame to the underlying writer
	n, err := w.w.WithContext(ctx).Write(frame)
	// Guard against short writes
	return catchShortWrite(n, err, frame)
}

func (w *delegatingWriter) Close(ctx context.Context) error { return w.w.WithContext(ctx).Close() }

func newErrWriter(ct content.ContentType, err error, meta content.Metadata) Writer {
	return &errWriter{
		meta.Clone().ToContainer(),
		ct,
		&nopCloser{},
		err,
	}
}

type errWriter struct {
	content.MetadataContainer
	content.ContentTyped
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
