package frame

import (
	"io"

	"github.com/weaveworks/libgitops/pkg/stream"
	"k8s.io/apimachinery/pkg/runtime/serializer/json"
)

func (defaultFactory) NewWriter(ct stream.ContentType, w stream.Writer, opts ...WriterOption) Writer {
	o := defaultWriterOptions().applyOptions(opts)

	var lowlevel Writer
	switch ct {
	case stream.ContentTypeYAML:
		lowlevel = newDelegatingWriter(stream.ContentTypeYAML, w.Wrap(func(underlying io.WriteCloser) io.Writer {
			// This writer always prepends a "---" before each frame
			return json.YAMLFramer.NewFrameWriter(underlying)
		}))
	case stream.ContentTypeJSON:
		// JSON documents are self-framing; hence, no need to wrap the writer in any way
		lowlevel = newDelegatingWriter(stream.ContentTypeJSON, w)
	default:
		return newErrWriter(ct, stream.ErrUnsupportedContentType(ct), w.ContentMetadata())
	}
	return newHighlevelWriter(lowlevel, o)
}

func (defaultFactory) NewSingleWriter(ct stream.ContentType, w stream.Writer, opts ...SingleWriterOption) Writer {
	o := defaultSingleWriterOptions().applyOptions(opts)

	return newHighlevelWriter(newDelegatingWriter(ct, w), &writerOptions{
		Options: Options{
			SingleOptions: o.SingleOptions,
			MaxFrameCount: 1,
		},
	})
}

func (f defaultFactory) NewRecognizingWriter(w stream.Writer, opts ...RecognizingWriterOption) Writer {
	o := defaultRecognizingWriterOptions().applyOptions(opts)

	// Recognize the content type using the given recognizer
	r, ct, err := stream.NewRecognizingWriter(w, o.Recognizer)
	if err != nil {
		return newErrWriter("", err, r.ContentMetadata())
	}
	// Re-use the logic of the "main" Writer constructor; validate ct there
	return f.NewWriter(ct, w, o)
}
