package serializer

import (
	"io"
)

const (
	yamlSeparator = "---\n"
)

// Writer in this package is an alias for io.Writer. It helps in Godoc to locate
// helpers in this package which returns writers (i.e. ToBytes)
type Writer io.Writer

// FrameWriter is a ContentType-specific io.Writer that writes given frames in an applicable way
// to an underlying io.Writer stream
type FrameWriter interface {
	ContentTyped
	Writer
}

// NewFrameWriter returns a new FrameWriter for the given Writer and ContentType
func NewFrameWriter(contentType ContentType, w Writer) FrameWriter {
	switch contentType {
	case ContentTypeYAML:
		// Use our own implementation of the underlying YAML FrameWriter
		return &frameWriter{newYAMLWriter(w), contentType}
	case ContentTypeJSON:
		// Comment from k8s.io/apimachinery/pkg/runtime/serializer/json.Framer.NewFrameWriter:
		// "we can write JSON objects directly to the writer, because they are self-framing"
		// Hence, we directly use w without any modifications.
		return &frameWriter{w, contentType}
	default:
		return &errFrameWriter{ErrUnsupportedContentType, contentType}
	}
}

// NewYAMLFrameWriter returns a FrameWriter that writes YAML frames separated by "---\n"
//
// This call is the same as NewFrameWriter(ContentTypeYAML, w)
func NewYAMLFrameWriter(w Writer) FrameWriter {
	return NewFrameWriter(ContentTypeYAML, w)
}

// NewJSONFrameWriter returns a FrameWriter that writes JSON frames without separation
// (i.e. "{ ... }{ ... }{ ... }" on the wire)
//
// This call is the same as NewFrameWriter(ContentTypeYAML, w)
func NewJSONFrameWriter(w Writer) FrameWriter {
	return NewFrameWriter(ContentTypeJSON, w)
}

// frameWriter is an implementation of the FrameWriter interface
type frameWriter struct {
	Writer

	contentType ContentType

	// TODO: Maybe add mutexes for thread-safety (so no two goroutines write at the same time)
}

// ContentType returns the content type for the given FrameWriter
func (wf *frameWriter) ContentType() ContentType {
	return wf.contentType
}

// newYAMLWriter returns a new yamlWriter implementation
func newYAMLWriter(w Writer) *yamlWriter {
	return &yamlWriter{
		w:          w,
		hasWritten: false,
	}
}

// yamlWriter writes yamlSeparator between documents
type yamlWriter struct {
	w          io.Writer
	hasWritten bool
}

// Write implements io.Writer
func (w *yamlWriter) Write(p []byte) (n int, err error) {
	// If we've already written some documents, add the separator in between
	if w.hasWritten {
		_, err = w.w.Write([]byte(yamlSeparator))
		if err != nil {
			return
		}
	}

	// Write the given bytes to the underlying writer
	n, err = w.w.Write(p)
	if err != nil {
		return
	}

	// Mark that we've now written once and should write the separator in between
	w.hasWritten = true
	return
}

// ToBytes returns a Writer which can be passed to NewFrameWriter. The Writer writes directly
// to an underlying byte array. The byte array must be of enough length in order to write.
func ToBytes(p []byte) Writer {
	return &byteWriter{p, 0}
}

type byteWriter struct {
	to []byte
	// the next index to write to
	index int
}

func (w *byteWriter) Write(from []byte) (n int, err error) {
	// Check if we have space in to, in order to write bytes there
	if w.index+len(from) > len(w.to) {
		err = io.ErrShortBuffer
		return
	}
	// Copy over the bytes one by one
	for i := range from {
		w.to[w.index+i] = from[i]
	}
	// Increase the index for the next Write call's target position
	w.index += len(from)
	n += len(from)
	return
}
