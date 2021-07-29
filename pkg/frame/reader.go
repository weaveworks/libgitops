package frame

import (
	"context"
	"sync"

	"github.com/weaveworks/libgitops/pkg/content"
	"github.com/weaveworks/libgitops/pkg/frame/sanitize"
	"github.com/weaveworks/libgitops/pkg/tracing"
	"github.com/weaveworks/libgitops/pkg/util/limitedio"
	"go.opentelemetry.io/otel/trace"
)

// newHighlevelReader takes a "low-level" Reader (like *streamingReader or *yamlReader),
// and implements higher-level logic like proper closing, mutex locking and tracing.
func newHighlevelReader(r Reader, o *readerOptions) Reader {
	return &highlevelReader{
		read:           r,
		readMu:         &sync.Mutex{},
		opts:           o,
		maxTotalFrames: limitedio.Limit(o.MaxFrameCount * 10),
	}
}

// highlevelReader uses the closableResource for the mutex locking, properly handling
// the close logic, and initiating the trace spans. On top of that it records extra
// tracing context in ReadFrame.
type highlevelReader struct {
	read Reader
	// readMu guards read.ReadFrame
	readMu *sync.Mutex

	opts *readerOptions
	// maxTotalFrames is set to opts.MaxFrameCount * 10
	maxTotalFrames limitedio.Limit
	// successfulFrameCount counts the amount of successful frames read
	successfulFrameCount int64
	// totalFrameCount counts the total amount of frames read (including empty and failed ones)
	totalFrameCount int64
}

func (r *highlevelReader) ReadFrame(ctx context.Context) ([]byte, error) {
	// Make sure we have access to the underlying resource
	r.readMu.Lock()
	defer r.readMu.Unlock()

	var frame []byte
	err := tracing.FromContext(ctx, r).
		TraceFunc(ctx, "ReadFrame", func(ctx context.Context, span trace.Span) error {

			// Refuse to read more than the maximum amount of successful frames
			if r.opts.MaxFrameCount.IsLessThan(r.successfulFrameCount) {
				return ErrFrameCountOverflow(r.opts.MaxFrameCount)
			}

			// Call the underlying reader
			var err error
			frame, err = r.readFrame(ctx)
			if err != nil {
				return err
			}

			// Record how large the frame is, and its content for debugging
			span.SetAttributes(content.SpanAttrByteContent(frame)...)
			return nil
		}).RegisterCustom(content.SpanRegisterReadError)
	// SpanRegisterReadError registers io.EOF as an "event", and other errors as "unknown errors" in the trace
	if err != nil {
		return nil, err
	}
	return frame, nil
}

func (r *highlevelReader) readFrame(ctx context.Context) ([]byte, error) {
	// Ensure the total number of frames doesn't overflow
	// TODO: Should this be LT or LTE?
	if r.maxTotalFrames.IsLessThanOrEqual(r.totalFrameCount) {
		return nil, ErrFrameCountOverflow(r.maxTotalFrames)
	}
	// Read the frame, and increase the total frame counter is increased
	// This does not at the moment forward the same ReadFrameResult instance,
	// but that can maybe be done in the future if needed. It would be needed
	// if the underlying Reader would return an interface that extends more
	// methods than the default ones.
	frame, err := r.read.ReadFrame(ctx)
	r.totalFrameCount += 1
	if err != nil {
		return nil, err
	}

	// Sanitize the frame.
	frame, err = sanitize.IfSupported(ctx, r.opts.Sanitizer, r.ContentType(), frame)
	if err != nil {
		return nil, err
	}

	// If it's empty, read the next frame automatically
	if len(frame) == 0 {
		return r.readFrame(ctx)
	}

	// Otherwise, if it's non-empty, return it and increase the "successful" counter
	r.successfulFrameCount += 1
	// If the frame count now overflows, return a ErrFrameCountOverflow
	if r.opts.MaxFrameCount.IsLessThan(r.successfulFrameCount) {
		return nil, ErrFrameCountOverflow(r.opts.MaxFrameCount)
	}
	return frame, nil
}

func (r *highlevelReader) ContentType() content.ContentType  { return r.read.ContentType() }
func (r *highlevelReader) Close(ctx context.Context) error   { return closeWithTrace(ctx, r.read, r) }
func (r *highlevelReader) ContentMetadata() content.Metadata { return r.read.ContentMetadata() }
