package frame

import (
	"io"

	"github.com/weaveworks/libgitops/pkg/content"
	"k8s.io/apimachinery/pkg/runtime/serializer/json"
)

func (defaultFactory) NewWriter(ct content.ContentType, w content.Writer, opts ...WriterOption) Writer {
	o := defaultWriterOptions().applyOptions(opts)

	var lowlevel Writer
	switch ct {
	case content.ContentTypeYAML:
		lowlevel = newDelegatingWriter(content.ContentTypeYAML, w.Wrap(func(underlying io.WriteCloser) io.Writer {
			// This writer always prepends a "---" before each frame
			return json.YAMLFramer.NewFrameWriter(underlying)
		}))
	case content.ContentTypeJSON:
		// JSON documents are self-framing; hence, no need to wrap the writer in any way
		lowlevel = newDelegatingWriter(content.ContentTypeJSON, w)
	default:
		return newErrWriter(ct, content.ErrUnsupportedContentType(ct), w.ContentMetadata())
	}
	return newHighlevelWriter(lowlevel, o)
}

func (defaultFactory) NewSingleWriter(ct content.ContentType, w content.Writer, opts ...SingleWriterOption) Writer {
	o := defaultSingleWriterOptions().applyOptions(opts)

	return newHighlevelWriter(newDelegatingWriter(ct, w), &writerOptions{
		Options: Options{
			SingleOptions: o.SingleOptions,
			MaxFrameCount: 1,
		},
	})
}

func (f defaultFactory) NewRecognizingWriter(w content.Writer, opts ...RecognizingWriterOption) Writer {
	o := defaultRecognizingWriterOptions().applyOptions(opts)

	// Recognize the content type using the given recognizer
	r, ct, err := content.NewRecognizingWriter(w, o.Recognizer)
	if err != nil {
		return newErrWriter("", err, r.ContentMetadata())
	}
	// Re-use the logic of the "main" Writer constructor; validate ct there
	return f.NewWriter(ct, w, o)
}
