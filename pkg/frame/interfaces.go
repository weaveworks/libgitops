package frame

import (
	"context"

	"github.com/weaveworks/libgitops/pkg/content"
)

// TODO: Maybe implement/use context-aware (cancellable) io.Readers and io.Writers underneath?

// Closer is like io.Closer, but with a Context passed along as well.
type Closer interface {
	// Close closes the underlying resource. If Close is called multiple times, the
	// underlying io.Closer decides the behavior and return value. If Close is called
	// during a Read/Write operation, the underlying io.ReadCloser/io.WriteCloser
	// decides the behavior.
	Close(ctx context.Context) error
}

// Reader is a framing type specific reader of an underlying io.Reader or io.ReadCloser.
// If an io.Reader is used, Close(ctx) is a no-op. If an io.ReadCloser is used, Close(ctx)
// will close the underlying io.ReadCloser.
//
// The Reader returns frames, as defined by the relevant framing type.
// For example, for YAML a frame represents a YAML document, while JSON is a self-framing
// format, i.e. encoded objects can be written to a stream just as
// '{ "a": "" ... }{ "b": "" ... }' and separated from there.
//
// Another way of defining a "frame" is that it MUST contain exactly one decodable object.
// This means that no empty (i.e. len(frame) == 0) frames shall be returned. Note: The decodable
// object might represent a list object (e.g. as Kubernetes' v1.List); more generally something
// decodable into a Go struct.
//
// The Reader can use as many underlying Read(p []byte) (n int, err error) calls it needs
// to the underlying io.Read(Clos)er. As long as frames can successfully be read from the underlying
// io.Read(Clos)er, len(frame) != 0 and err == nil. When io.EOF is encountered, len(frame) == 0 and
// errors.Is(err, io.EOF) == true.
//
// The Reader MUST be thread-safe, i.e. it must use the underlying io.Reader responsibly
// without causing race conditions when reading, e.g. by guarding reads with a mutual
// exclusion lock (mutex). The mutex isn't locked for closes, however. This enables e.g. closing the
// reader during a read operation, and other custom closing behaviors.
//
// The Reader MUST directly abort the read operation if the frame size exceeds
// ReadWriterOptions.MaxFrameSize, and return ErrFrameSizeOverflow.
//
// The Reader MUST return ErrFrameCountOverflow if the underlying Reader has returned more than
// ReadWriterOptions.MaxFrameCount successful read operations. The "total" frame limit is
// 10 * ReadWriterOptions.MaxFrameCount, which includes failed, empty and successful frames.
// Returned errors (including io.EOF) MUST be checked for equality using
// errors.Is(err, target), NOT using err == target.
//
// TODO: Say that the ContentType is assumed constant per content.Reader
//
// The Reader MAY respect cancellation signals on the context, depending on ReaderOptions.
// The Reader MAY support reporting trace spans for how long certain operations take.
type Reader interface {
	// The Reader is specific to possibly multiple framing types
	content.ContentTyped

	// ReadFrame reads one frame from the underlying io.Read(Clos)er. At maximum, the frame is as
	// large as ReadWriterOptions.MaxFrameSize. See the documentation on the Reader interface for more
	// details.
	ReadFrame(ctx context.Context) ([]byte, error)

	// Exposes Metadata about the underlying io.Reader
	content.MetadataContainer

	// The Reader can be closed. If an underlying io.Reader is used, this is a no-op. If an
	// io.ReadCloser is used, this will close that io.ReadCloser.
	Closer
}

type ReaderFactory interface {
	// ct is dominant; will error if r has a conflicting content type
	// ct must be one of the supported content types
	NewReader(ct content.ContentType, r content.Reader, opts ...ReaderOption) Reader
	// opts.MaxFrameCount is dominant, will always be set to 1
	// ct can be anything
	// ct is dominant; will error if r has a conflicting content type
	// Single options should not have MaxFrameCount at all, if possible
	NewSingleReader(ct content.ContentType, r content.Reader, opts ...SingleReaderOption) Reader
	// will use the content type from r if set, otherwise infer from content metadata
	// or peek bytes using the content.ContentTypeRecognizer
	// should add to options for a recognizer
	NewRecognizingReader(ctx context.Context, r content.Reader, opts ...RecognizingReaderOption) Reader

	//SupportedContentTypes()
}

// Writer is a framing type specific writer to an underlying io.Writer or io.WriteCloser.
// If an io.Writer is used, Close(ctx) is a no-op. If an io.WriteCloser is used, Close(ctx)
// will close the underlying io.WriteCloser.
//
// The Writer writes frames to the underlying stream, as defined by the framing type.
// For example, for YAML a frame represents a YAML document, while JSON is a self-framing
// format, i.e. encoded objects can be written to a stream just as
// '{ "a": "" ... }{ "b": "" ... }'.
//
// Another way of defining a "frame" is that it MUST contain exactly one decodable object.
// It is valid (but not recommended) to supply empty frames to the Writer.
//
// Writer will only call the underlying io.Write(Close)r's Write(p []byte) call once.
// If n < len(frame) and err == nil, io.ErrShortWrite will be returned. This means that
// it's the underlying io.Writer's responsibility to buffer the frame data, if needed.
//
// The Writer MUST be thread-safe, i.e. it must use the underlying io.Writer responsibly
// without causing race conditions when reading, e.g. by guarding writes/closes with a
// mutual exclusion lock (mutex). The mutex isn't locked for closes, however.
// This enables e.g. closing the writer during a write operation, and other custom closing behaviors.
//
// The Writer MUST directly abort the write operation if the frame size exceeds ReadWriterOptions.MaxFrameSize,
// and return ErrFrameSizeOverflow. The Writer MUST ignore empty frames, where len(frame) == 0, possibly
// after sanitation. The Writer MUST return ErrFrameCountOverflow if WriteFrame has been called more than
// ReadWriterOptions.MaxFrameCount times.
//
// Returned errors MUST be checked for equality using errors.Is(err, target), NOT using err == target.
//
// The Writer MAY respect cancellation signals on the context, depending on WriterOptions.
// The Writer MAY support reporting trace spans for how long certain operations take.
//
// TODO: Say that the ContentType is assumed constant per content.Writer
type Writer interface {
	// The Writer is specific to this framing type.
	content.ContentTyped
	// WriteFrame writes one frame to the underlying io.Write(Close)r.
	// See the documentation on the Writer interface for more details.
	WriteFrame(ctx context.Context, frame []byte) error

	// Exposes metadata from the underlying content.Writer
	content.MetadataContainer

	// The Writer can be closed. If an underlying io.Writer is used, this is a no-op. If an
	// io.WriteCloser is used, this will close that io.WriteCloser.
	Closer
}

type WriterFactory interface {
	// ct is dominant; will error if r has a conflicting content type
	// ct must be one of the supported content types
	NewWriter(ct content.ContentType, w content.Writer, opts ...WriterOption) Writer
	// opts.MaxFrameCount is dominant, will always be set to 1
	// ct can be anything
	// ct is dominant; will error if r has a conflicting content type
	// Single options should not have MaxFrameCount at all, if possible
	NewSingleWriter(ct content.ContentType, w content.Writer, opts ...SingleWriterOption) Writer
	// will use the content type from r if set, otherwise infer from content metadata
	// using the content.ContentTypeRecognizer
	// should add to options for a recognizer
	NewRecognizingWriter(w content.Writer, opts ...RecognizingWriterOption) Writer

	// The SupportedContentTypes() method specifies what content types are supported by the
	// NewWriter
	content.ContentTypeSupporter
}

type Factory interface {
	ReaderFactory
	WriterFactory
}
