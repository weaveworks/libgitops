package frame

import (
	"bytes"
	"context"

	"github.com/weaveworks/libgitops/pkg/content"
)

// 2 generic Reader constructors

func NewSingleReader(ct content.ContentType, r content.Reader, opts ...SingleReaderOption) Reader {
	return internalFactoryVar.NewSingleReader(ct, r, opts...)
}

func NewRecognizingReader(ctx context.Context, r content.Reader, opts ...RecognizingReaderOption) Reader {
	return internalFactoryVar.NewRecognizingReader(ctx, r, opts...)
}

// 4 JSON-YAML Reader constructors using the default factory

func NewYAMLReader(r content.Reader, opts ...ReaderOption) Reader {
	return internalFactoryVar.NewReader(content.ContentTypeYAML, r, opts...)
}

func NewJSONReader(r content.Reader, opts ...ReaderOption) Reader {
	return internalFactoryVar.NewReader(content.ContentTypeJSON, r, opts...)
}

func NewSingleYAMLReader(r content.Reader, opts ...SingleReaderOption) Reader {
	return NewSingleReader(content.ContentTypeYAML, r, opts...)
}

func NewSingleJSONReader(r content.Reader, opts ...SingleReaderOption) Reader {
	return NewSingleReader(content.ContentTypeJSON, r, opts...)
}

// 2 generic Writer constructors

func NewSingleWriter(ct content.ContentType, w content.Writer, opts ...SingleWriterOption) Writer {
	return internalFactoryVar.NewSingleWriter(ct, w, opts...)
}

func NewRecognizingWriter(r content.Writer, opts ...RecognizingWriterOption) Writer {
	return internalFactoryVar.NewRecognizingWriter(r, opts...)
}

// 4 JSON-YAML Writer constructors using the default factory

func NewYAMLWriter(r content.Writer, opts ...WriterOption) Writer {
	return internalFactoryVar.NewWriter(content.ContentTypeYAML, r, opts...)
}

func NewJSONWriter(r content.Writer, opts ...WriterOption) Writer {
	return internalFactoryVar.NewWriter(content.ContentTypeJSON, r, opts...)
}

func NewSingleYAMLWriter(r content.Writer, opts ...SingleWriterOption) Writer {
	return internalFactoryVar.NewSingleWriter(content.ContentTypeYAML, r, opts...)
}

func NewSingleJSONWriter(r content.Writer, opts ...SingleWriterOption) Writer {
	return internalFactoryVar.NewSingleWriter(content.ContentTypeJSON, r, opts...)
}

// 1 single, 3 YAML and 1 recognizing content.Reader helper constructors

/*func FromSingleBuffer(ct content.ContentType, buf *bytes.Buffer, opts ...SingleReaderOption) Reader {
	return NewSingleReader(ct, content.FromBuffer(buf), opts...)
}*/

func FromYAMLBytes(yamlBytes []byte, opts ...ReaderOption) Reader {
	return NewYAMLReader(content.FromBytes(yamlBytes), opts...)
}

func FromYAMLString(yamlStr string, opts ...ReaderOption) Reader {
	return NewYAMLReader(content.FromString(yamlStr), opts...)
}

func FromYAMLFile(filePath string, opts ...ReaderOption) Reader {
	return NewYAMLReader(content.FromFile(filePath), opts...)
}

func FromFile(ctx context.Context, filePath string, opts ...RecognizingReaderOption) Reader {
	return NewRecognizingReader(ctx, content.FromFile(filePath), opts...)
}

// 1 single, 2 YAML and 1 recognizing content.Writer helper constructors

func ToSingleBuffer(ct content.ContentType, buf *bytes.Buffer, opts ...SingleWriterOption) Writer {
	return NewSingleWriter(ct, content.ToBuffer(buf), opts...)
}

func ToYAMLBuffer(buf *bytes.Buffer, opts ...WriterOption) Writer {
	return NewYAMLWriter(content.NewWriter(buf), opts...)
}

func ToYAMLFile(filePath string, opts ...WriterOption) Writer {
	return NewYAMLWriter(content.ToFile(filePath), opts...)
}

func ToFile(filePath string, opts ...RecognizingWriterOption) Writer {
	return NewRecognizingWriter(content.ToFile(filePath), opts...)
}
