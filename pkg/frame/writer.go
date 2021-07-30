package frame

import (
	"context"
	"sync"

	"github.com/weaveworks/libgitops/pkg/frame/sanitize"
	"github.com/weaveworks/libgitops/pkg/stream"
	"github.com/weaveworks/libgitops/pkg/tracing"
	"github.com/weaveworks/libgitops/pkg/util/limitedio"
	"go.opentelemetry.io/otel/trace"
)

func newHighlevelWriter(w Writer, opts *writerOptions) Writer {
	return &highlevelWriter{
		writer:   w,
		writerMu: &sync.Mutex{},
		opts:     opts,
	}
}

type highlevelWriter struct {
	writer   Writer
	writerMu *sync.Mutex
	opts     *writerOptions
	// frameCount counts the amount of successful frames written
	frameCount int64
}

func (w *highlevelWriter) WriteFrame(ctx context.Context, frame []byte) error {
	w.writerMu.Lock()
	defer w.writerMu.Unlock()

	return tracing.FromContext(ctx, w).TraceFunc(ctx, "WriteFrame", func(ctx context.Context, span trace.Span) error {
		// Refuse to write too large frames
		if w.opts.MaxFrameSize.IsLessThan(int64(len(frame))) {
			return limitedio.ErrReadSizeOverflow(w.opts.MaxFrameSize)
		}
		// Refuse to write more than the maximum amount of frames
		if w.opts.MaxFrameCount.IsLessThanOrEqual(w.frameCount) {
			return ErrFrameCountOverflow(w.opts.MaxFrameCount)
		}

		// Sanitize the frame
		// TODO: Maybe create a composite writer that actually reads the given frame first, to
		// fully sanitize/validate it, and first then write the frames out using the writer?
		frame, err := sanitize.IfSupported(ctx, w.opts.Sanitizer, w.ContentType(), frame)
		if err != nil {
			return err
		}

		// Register the amount of (sanitized) bytes and call the underlying Writer
		span.SetAttributes(stream.SpanAttrByteContent(frame)...)

		// Catch empty frames
		if len(frame) == 0 {
			return nil
		}

		err = w.writer.WriteFrame(ctx, frame)

		// Increase the frame counter, if the write was successful
		if err == nil {
			w.frameCount += 1
		}
		return err
	}).Register()
}

func (w *highlevelWriter) ContentType() stream.ContentType { return w.writer.ContentType() }
func (w *highlevelWriter) Close(ctx context.Context) error {
	return closeWithTrace(ctx, w.writer, w)
}

// Just forward the metadata, don't do anything specific with it
func (w *highlevelWriter) ContentMetadata() stream.Metadata { return w.writer.ContentMetadata() }
