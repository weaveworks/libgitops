package frame

import (
	"context"
	"errors"
	"io"

	"github.com/weaveworks/libgitops/pkg/stream"
	"github.com/weaveworks/libgitops/pkg/util/limitedio"
	"k8s.io/apimachinery/pkg/runtime/serializer/json"
	"k8s.io/apimachinery/pkg/runtime/serializer/streaming"
)

func newYAMLReader(r stream.Reader, o *readerOptions) Reader {
	// json.YAMLFramer.NewFrameReader takes care of the actual YAML framing logic
	maxFrameSizeInt, err := o.MaxFrameSize.Int()
	if err != nil {
		return newErrReader(err, "", r.ContentMetadata())
	}
	r = r.Wrap(func(underlying io.ReadCloser) io.Reader {
		return newK8sYAMLReader(underlying, maxFrameSizeInt)
	})

	// Mark the content type as YAML
	r.ContentMetadata().Apply(stream.WithContentType(stream.ContentTypeYAML))

	return newStreamingReader(stream.ContentTypeYAML, r, o.MaxFrameSize)
}

// newJSONReader creates a "low-level" JSON Reader from the given io.ReadCloser.
func newJSONReader(r stream.Reader, o *readerOptions) Reader {
	// json.Framer.NewFrameReader takes care of the actual JSON framing logic
	r = r.Wrap(func(underlying io.ReadCloser) io.Reader {
		return json.Framer.NewFrameReader(underlying)
	})

	// Mark the content type as JSON
	r.ContentMetadata().Apply(stream.WithContentType(stream.ContentTypeJSON))

	return newStreamingReader(stream.ContentTypeJSON, r, o.MaxFrameSize)
}

// newStreamingReader makes a generic Reader that reads from an io.ReadCloser returned
// from Kubernetes' runtime.Framer.NewFrameReader, in exactly the way
// k8s.io/apimachinery/pkg/runtime/serializer/streaming implements this.
// On a high-level, it means that many small Read(p []byte) calls are made as long as
// io.ErrShortBuffer is returned. When err == nil is returned from rc, we know that we're
// at the end of a frame, and at that point the frame is returned.
//
// Note: This Reader is a so-called "low-level" one. It doesn't do tracing, mutex locking, or
// proper closing logic. It must be wrapped by a composite, high-level Reader like highlevelReader.
func newStreamingReader(ct stream.ContentType, r stream.Reader, maxFrameSize limitedio.Limit) Reader {
	// Limit the amount of bytes read from the stream.Reader
	r, resetCounter := stream.WrapLimited(r, maxFrameSize)
	// Wrap
	cr := r.WrapSegment(func(rc io.ReadCloser) stream.RawSegmentReader {
		return newK8sStreamingReader(rc, maxFrameSize.Int64())
	})

	return &streamingReader{
		// Clone the metadata and expose it
		// TODO: Maybe ReaderOptions should allow changing it?
		MetadataContainer: r.ContentMetadata().Clone().ToContainer(),
		ContentTyped:      ct,
		resetCounter:      resetCounter,
		cr:                cr,
		maxFrameSize:      maxFrameSize,
	}
}

// streamingReader is a small "conversion" struct that implements the Reader interface for a
// given k8sStreamingReader. When reader_streaming_k8s.go is upstreamed, we can replace the
// temporary k8sStreamingReader interface with a "proper" Kubernetes one.
type streamingReader struct {
	stream.MetadataContainer
	stream.ContentTyped
	resetCounter stream.ResetCounterFunc
	cr           stream.SegmentReader
	maxFrameSize limitedio.Limit
}

func (r *streamingReader) ReadFrame(ctx context.Context) ([]byte, error) {
	// Read one frame from the streamReader
	frame, err := r.cr.WithContext(ctx).Read()
	if err != nil {
		// Transform streaming.ErrObjectTooLarge to a ErrFrameSizeOverflow, if returned.
		return nil, mapError(err, errorMappings{
			streaming.ErrObjectTooLarge: func() error {
				return limitedio.ErrReadSizeOverflow(r.maxFrameSize)
			},
		})
	}
	// Reset the counter only when we have a successful frame
	r.resetCounter()
	return frame, nil
}

func (r *streamingReader) Close(ctx context.Context) error { return r.cr.WithContext(ctx).Close() }

// mapError is an utility for mapping a "actual" error to a lazily-evaluated "desired" one.
// Equality between the errorMappings' keys and err is defined by errors.Is
func mapError(err error, f errorMappings) error {
	for target, mkErr := range f {
		if errors.Is(err, target) {
			return mkErr()
		}
	}
	return err
}

// errorMappings maps actual errors to lazily-evaluated desired ones
type errorMappings map[error]mkErrorFunc

// mkErrorFunc lazily creates an error
type mkErrorFunc func() error
