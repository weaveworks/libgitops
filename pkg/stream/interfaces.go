package stream

import (
	"context"
	"fmt"
	"io"

	"github.com/weaveworks/libgitops/pkg/stream/metadata"
)

var _ fmt.Stringer = ContentType("")

// ContentType specifies the content type of some stream.
// Ideally, a standard MIME notation like "application/json" shall be used.
type ContentType string

const (
	ContentTypeYAML ContentType = "application/yaml"
	ContentTypeJSON ContentType = "application/json"
)

func (ct ContentType) ContentType() ContentType { return ct }
func (ct ContentType) String() string           { return string(ct) }

type ContentTypes []ContentType

func (cts ContentTypes) Has(want ContentType) bool {
	for _, ct := range cts {
		if ct == want {
			return true
		}
	}
	return false
}

func WithContentType(ct ContentType) metadata.HeaderOption {
	return metadata.SetOption(metadata.ContentTypeKey, ct.String())
}

// ContentTyped is an interface that contains and/or supports one content type.
type ContentTyped interface {
	ContentType() ContentType
}

// ContentTypeSupporter supports potentially multiple content types.
type ContentTypeSupporter interface {
	// Order _might_ carry a meaning
	SupportedContentTypes() ContentTypes
}

// underlying is the underlying stream of the Reader.
// If the returned io.Reader does not implement io.Closer,
// the underlying.Close() method will be re-used.
type WrapReaderFunc func(underlying io.ReadCloser) io.Reader

type WrapWriterFunc func(underlying io.WriteCloser) io.Writer

type WrapReaderToSegmentFunc func(underlying io.ReadCloser) RawSegmentReader

// TODO: More documentation on these types.

// Reader is a tracing-capable and metadata-bound io.Reader and io.Closer
// wrapper. It is NOT thread-safe by default. It supports introspection
// of composite ReadClosers. The TracerProvider from the given context
// is used.
//
// The Reader reads the current span from the given context, and uses that
// span's TracerProvider to create a Tracer and then also a new Span for
// the current operation.
type Reader interface {
	// These call the underlying Set/ClearContext functions before/after
	// reads and closes, and then uses the underlying io.ReadCloser.
	// If the underlying Reader doesn't support closing, the returned
	// Close method will only log a "CloseNoop" trace and exit with err == nil.
	WithContext(ctx context.Context) io.ReadCloser

	// This reader supports registering metadata about the content it
	// is reading.
	MetadataContainer

	// Wrap returns a new Reader with io.ReadCloser B that reads from
	// the current Reader's underlying io.ReadCloser A. If the returned
	// B is an io.ReadCloser or this Reader's HasCloser() is true,
	// HasCloser() of the returned Reader will be true, otherwise false.
	Wrap(fn WrapReaderFunc) Reader
	WrapSegment(fn WrapReaderToSegmentFunc) SegmentReader
}

type RawSegmentReader interface {
	Read() ([]byte, error)
}

type ClosableRawSegmentReader interface {
	RawSegmentReader
	io.Closer
}

type SegmentReader interface {
	WithContext(ctx context.Context) ClosableRawSegmentReader

	MetadataContainer
}

// In the future, one can implement a WrapSegment function that is of
// the following form:
// WrapSegment(name string, fn WrapSegmentFunc) SegmentReader
// where WrapSegmentFunc is func(underlying ClosableRawSegmentReader) RawSegmentReader
// This allows chaining simple composite SegmentReaders

type Writer interface {
	WithContext(ctx context.Context) io.WriteCloser

	// This writer supports registering metadata about the content it
	// is writing and the destination it is writing to.
	MetadataContainer

	Wrap(fn WrapWriterFunc) Writer
}

type readerInternal interface {
	Reader
	RawReader() io.Reader
	RawCloser() io.Closer
}

type segmentReaderInternal interface {
	SegmentReader
	RawSegmentReader() RawSegmentReader
	RawCloser() io.Closer
}

type writerInternal interface {
	Writer
	RawWriter() io.Writer
	RawCloser() io.Closer
}

// The internal implementation structs should implement the
// ...Internal interfaces, in order to expose their raw, underlying resources
// just in case it is _really_ needed upstream (e.g. for testing). It is not
// exposed by default in the interface to avoid showing up in Godoc, as it
// most often shouldn't be used.
var _ readerInternal = &reader{}
var _ segmentReaderInternal = &segmentReader{}
var _ writerInternal = &writer{}
