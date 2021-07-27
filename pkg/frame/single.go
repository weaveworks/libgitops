package frame

import (
	"context"
	"io"

	"github.com/weaveworks/libgitops/pkg/content"
)

func newSingleReader(r content.Reader, ct content.ContentType, o *singleReaderOptions) Reader {
	// Make sure not more than this set of bytes can be read
	r, _ = content.WrapLimited(r, o.MaxFrameSize)
	return &singleReader{
		// TODO: Apply options?
		MetadataContainer: r.ContentMetadata().Clone().ToContainer(),
		ContentTyped:      ct,
		r:                 r,
	}
}

// singleReader implements reading a single frame (up to a certain limit) from an io.ReadCloser.
// It MUST be wrapped in a higher-level composite Reader like the highlevelReader to satisfy the
// Reader interface correctly.
type singleReader struct {
	content.MetadataContainer
	content.ContentTyped
	r           content.Reader
	hasBeenRead bool
}

// Read the whole frame from the underlying io.Reader, up to a given limit
func (r *singleReader) ReadFrame(ctx context.Context) ([]byte, error) {
	if r.hasBeenRead {
		// This really should never happen, because the higher-level Reader should ensure
		// no more than one frame can be read from the downstream as opts.MaxFrameCount == 1.
		return nil, io.EOF // TODO: What about the third time?
	}
	// Mark we are now the frame (regardless of the result)
	r.hasBeenRead = true
	// Read the whole frame from the underlying io.Reader, up to a given amount
	frame, err := io.ReadAll(r.r.WithContext(ctx))
	if err != nil {
		return nil, err
	}
	return frame, nil
}

func (r *singleReader) Close(ctx context.Context) error { return r.r.WithContext(ctx).Close() }
