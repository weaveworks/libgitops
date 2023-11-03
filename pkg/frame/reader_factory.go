package frame

import (
	"context"

	"github.com/weaveworks/libgitops/pkg/stream"
	"github.com/weaveworks/libgitops/pkg/util/limitedio"
)

// DefaultFactory returns the default implementation of both the ReaderFactory and
// WriterFactory. All constructors in this package use this Factory.
func DefaultFactory() Factory { return defaultFactory{} }

var internalFactoryVar = DefaultFactory()

type defaultFactory struct{}

func (defaultFactory) NewReader(ct stream.ContentType, r stream.Reader, opts ...ReaderOption) Reader {
	o := defaultReaderOptions().applyOptions(opts)

	var lowlevel Reader
	switch ct {
	case stream.ContentTypeYAML:
		lowlevel = newYAMLReader(r, o)
	case stream.ContentTypeJSON:
		lowlevel = newJSONReader(r, o)
	default:
		return newErrReader(stream.ErrUnsupportedContentType(ct), "", r.ContentMetadata())
	}
	return newHighlevelReader(lowlevel, o)
}

func (defaultFactory) NewSingleReader(ct stream.ContentType, r stream.Reader, opts ...SingleReaderOption) Reader {
	o := defaultSingleReaderOptions().applyOptions(opts)

	return newHighlevelReader(newSingleReader(r, ct, o), &readerOptions{
		// Note: The MaxFrameCount == Infinite here makes the singleReader responsible for
		// counting how many times
		Options: Options{SingleOptions: o.SingleOptions, MaxFrameCount: limitedio.Infinite},
	})
}

func (f defaultFactory) NewRecognizingReader(ctx context.Context, r stream.Reader, opts ...RecognizingReaderOption) Reader {
	o := defaultRecognizingReaderOptions().applyOptions(opts)

	// Recognize the content type using the given recognizer
	r, ct, err := stream.NewRecognizingReader(ctx, r, o.Recognizer)
	if err != nil {
		return newErrReader(err, "", r.ContentMetadata())
	}
	// Re-use the logic of the "main" Reader constructor; validate ct there
	return f.NewReader(ct, r, o)
}

func (defaultFactory) SupportedContentTypes() stream.ContentTypes {
	return []stream.ContentType{stream.ContentTypeYAML, stream.ContentTypeJSON}
}

func newErrReader(err error, ct stream.ContentType, meta stream.Metadata) Reader {
	return &errReader{
		ct,
		meta.ToContainer(),
		&nopCloser{},
		err,
	}
}

// errReader always returns an error
type errReader struct {
	stream.ContentTyped
	stream.MetadataContainer
	Closer
	err error
}

func (r *errReader) ReadFrame(context.Context) ([]byte, error) { return nil, r.err }
