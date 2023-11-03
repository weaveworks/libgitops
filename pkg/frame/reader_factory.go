package frame

import (
	"context"

	"github.com/weaveworks/libgitops/pkg/content"
	"github.com/weaveworks/libgitops/pkg/util/limitedio"
)

func DefaultFactory() Factory { return defaultFactory{} }

var internalFactoryVar = DefaultFactory()

type defaultFactory struct{}

func (defaultFactory) NewReader(ct content.ContentType, r content.Reader, opts ...ReaderOption) Reader {
	o := defaultReaderOptions().applyOptions(opts)

	var lowlevel Reader
	switch ct {
	case content.ContentTypeYAML:
		lowlevel = newYAMLReader(r, o)
	case content.ContentTypeJSON:
		lowlevel = newJSONReader(r, o)
	default:
		return newErrReader(content.ErrUnsupportedContentType(ct), "", r.ContentMetadata())
	}
	return newHighlevelReader(lowlevel, o)
}

func (defaultFactory) NewSingleReader(ct content.ContentType, r content.Reader, opts ...SingleReaderOption) Reader {
	o := defaultSingleReaderOptions().applyOptions(opts)

	return newHighlevelReader(newSingleReader(r, ct, o), &readerOptions{
		// Note: The MaxFrameCount == Infinite here makes the singleReader responsible for
		// counting how many times
		Options: Options{SingleOptions: o.SingleOptions, MaxFrameCount: limitedio.Infinite},
	})
}

func (f defaultFactory) NewRecognizingReader(ctx context.Context, r content.Reader, opts ...RecognizingReaderOption) Reader {
	o := defaultRecognizingReaderOptions().applyOptions(opts)

	// Recognize the content type using the given recognizer
	r, ct, err := content.NewRecognizingReader(ctx, r, o.Recognizer)
	if err != nil {
		return newErrReader(err, "", r.ContentMetadata())
	}
	// Re-use the logic of the "main" Reader constructor; validate ct there
	return f.NewReader(ct, r, o)
}

func (defaultFactory) SupportedContentTypes() content.ContentTypes {
	return []content.ContentType{content.ContentTypeYAML, content.ContentTypeJSON}
}

func newErrReader(err error, ct content.ContentType, meta content.Metadata) Reader {
	return &errReader{
		ct,
		meta.ToContainer(),
		&nopCloser{},
		err,
	}
}

// errReader always returns an error
type errReader struct {
	content.ContentTyped
	content.MetadataContainer
	Closer
	err error
}

func (r *errReader) ReadFrame(context.Context) ([]byte, error) { return nil, r.err }
