package frame

import (
	"bytes"
	"context"

	"github.com/weaveworks/libgitops/pkg/stream"
)

// 2 generic Reader constructors

func NewSingleReader(ct stream.ContentType, r stream.Reader, opts ...SingleReaderOption) Reader {
	return internalFactoryVar.NewSingleReader(ct, r, opts...)
}

func NewRecognizingReader(ctx context.Context, r stream.Reader, opts ...RecognizingReaderOption) Reader {
	return internalFactoryVar.NewRecognizingReader(ctx, r, opts...)
}

// 4 JSON-YAML Reader constructors using the default factory

func NewYAMLReader(r stream.Reader, opts ...ReaderOption) Reader {
	return internalFactoryVar.NewReader(stream.ContentTypeYAML, r, opts...)
}

func NewJSONReader(r stream.Reader, opts ...ReaderOption) Reader {
	return internalFactoryVar.NewReader(stream.ContentTypeJSON, r, opts...)
}

func NewSingleYAMLReader(r stream.Reader, opts ...SingleReaderOption) Reader {
	return NewSingleReader(stream.ContentTypeYAML, r, opts...)
}

func NewSingleJSONReader(r stream.Reader, opts ...SingleReaderOption) Reader {
	return NewSingleReader(stream.ContentTypeJSON, r, opts...)
}

// 2 generic Writer constructors

func NewSingleWriter(ct stream.ContentType, w stream.Writer, opts ...SingleWriterOption) Writer {
	return internalFactoryVar.NewSingleWriter(ct, w, opts...)
}

func NewRecognizingWriter(r stream.Writer, opts ...RecognizingWriterOption) Writer {
	return internalFactoryVar.NewRecognizingWriter(r, opts...)
}

// 4 JSON-YAML Writer constructors using the default factory

func NewYAMLWriter(r stream.Writer, opts ...WriterOption) Writer {
	return internalFactoryVar.NewWriter(stream.ContentTypeYAML, r, opts...)
}

func NewJSONWriter(r stream.Writer, opts ...WriterOption) Writer {
	return internalFactoryVar.NewWriter(stream.ContentTypeJSON, r, opts...)
}

func NewSingleYAMLWriter(r stream.Writer, opts ...SingleWriterOption) Writer {
	return internalFactoryVar.NewSingleWriter(stream.ContentTypeYAML, r, opts...)
}

func NewSingleJSONWriter(r stream.Writer, opts ...SingleWriterOption) Writer {
	return internalFactoryVar.NewSingleWriter(stream.ContentTypeJSON, r, opts...)
}

// 1 single, 3 YAML and 1 recognizing stream.Reader helper constructors

/*func FromSingleBuffer(ct stream.ContentType, buf *bytes.Buffer, opts ...SingleReaderOption) Reader {
	return NewSingleReader(ct, stream.FromBuffer(buf), opts...)
}*/

func FromYAMLBytes(yamlBytes []byte, opts ...ReaderOption) Reader {
	return NewYAMLReader(stream.FromBytes(yamlBytes), opts...)
}

func FromYAMLString(yamlStr string, opts ...ReaderOption) Reader {
	return NewYAMLReader(stream.FromString(yamlStr), opts...)
}

func FromYAMLFile(filePath string, opts ...ReaderOption) Reader {
	return NewYAMLReader(stream.FromFile(filePath), opts...)
}

func FromFile(ctx context.Context, filePath string, opts ...RecognizingReaderOption) Reader {
	return NewRecognizingReader(ctx, stream.FromFile(filePath), opts...)
}

// 1 single, 2 YAML and 1 recognizing stream.Writer helper constructors

func ToSingleBuffer(ct stream.ContentType, buf *bytes.Buffer, opts ...SingleWriterOption) Writer {
	return NewSingleWriter(ct, stream.ToBuffer(buf), opts...)
}

func ToYAMLBuffer(buf *bytes.Buffer, opts ...WriterOption) Writer {
	return NewYAMLWriter(stream.NewWriter(buf), opts...)
}

func ToYAMLFile(filePath string, opts ...WriterOption) Writer {
	return NewYAMLWriter(stream.ToFile(filePath), opts...)
}

func ToFile(filePath string, opts ...RecognizingWriterOption) Writer {
	return NewRecognizingWriter(stream.ToFile(filePath), opts...)
}
