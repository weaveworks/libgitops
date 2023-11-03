package content

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"io"
	"path/filepath"

	"github.com/weaveworks/libgitops/pkg/content/metadata"
	"github.com/weaveworks/libgitops/pkg/tracing"
	"github.com/weaveworks/libgitops/pkg/util/compositeio"
	"go.opentelemetry.io/otel/trace"
	yamlutil "k8s.io/apimachinery/pkg/util/yaml"
	"sigs.k8s.io/yaml"
)

const peekSize = 2048

type ContentTypeRecognizer interface {
	FromContentMetadata(m Metadata) (ct ContentType, ok bool)
	FromPeekBytes(peek []byte) (ct ContentType, ok bool)

	// SupportedContentTypes() tells about what ContentTypes are supported by this recognizer
	ContentTypeSupporter
}

func NewJSONYAMLRecognizingReader(ctx context.Context, r Reader) (Reader, ContentType, error) {
	return NewRecognizingReader(ctx, r, NewJSONYAMLContentTypeRecognizer())
}

func NewRecognizingReader(ctx context.Context, r Reader, ctrec ContentTypeRecognizer) (Reader, ContentType, error) {
	// If r already has Content-Type set, all good
	meta := r.ContentMetadata()
	ct, ok := meta.ContentType()
	if ok {
		return r, ct, nil
	}

	// Try to resolve the Content-Type from the X-Content-Location header
	ct, ok = ctrec.FromContentMetadata(meta)
	if ok {
		meta.Apply(WithContentType(ct))
		return r, ct, nil
	}

	var newr Reader
	err := tracing.FromContext(ctx, "content").TraceFunc(ctx, "NewRecognizingReader",
		func(ctx context.Context, span trace.Span) error {

			// Use the context to access the io.ReadCloser
			rc := r.WithContext(ctx)
			meta := r.ContentMetadata().Clone()

			bufr := bufio.NewReaderSize(rc, peekSize)

			peek, err := bufr.Peek(peekSize)
			if err != nil && !errors.Is(err, io.EOF) {
				return err
			}

			// Write to ct defined earlier, that value will be returned if err == nil
			ct, ok = ctrec.FromPeekBytes(peek)
			if !ok {
				// TODO: Struct error; include the peek in the context too
				return errors.New("couldn't recognize content type")
			}

			// Set the right recognized content type
			meta.Apply(WithContentType(ct))

			// Read from the buffered bufio.Reader, because we have already peeked
			// data from the underlying rc. Close rc when done.
			newr = NewReader(compositeio.ReadCloser(bufr, rc), meta)
			return nil
		}).Register()
	if err != nil {
		return nil, "", err
	}

	return newr, ct, nil
}

func NewRecognizingWriter(w Writer, ctrec ContentTypeRecognizer) (Writer, ContentType, error) {
	// If r already has Content-Type set, all good
	meta := w.ContentMetadata()
	ct, ok := meta.ContentType()
	if ok {
		return w, ct, nil
	}

	// Try to resolve the Content-Type from the X-Content-Location header
	ct, ok = ctrec.FromContentMetadata(meta)
	if ok {
		meta.Apply(WithContentType(ct))
		return w, ct, nil
	}

	// Negotiate the Accept header
	ct, ok = negotiateAccept(meta, ctrec.SupportedContentTypes())
	if ok {
		meta.Apply(WithContentType(ct))
		return w, ct, nil
	}

	return nil, "", errors.New("couldn't recognize content type")
}

const acceptAll ContentType = "*/*"

func negotiateAccept(meta Metadata, supportedTypes []ContentType) (ContentType, bool) {
	accepts, err := metadata.GetMediaTypes(meta, metadata.AcceptKey)
	if err != nil {
		return "", false
	}

	// prioritize the order that the metadata is asking for. supported is in priority order too
	for _, accept := range accepts {
		for _, supported := range supportedTypes {
			if matchesAccept(ContentType(accept), supported) {
				return supported, true
			}
		}
	}
	return "", false
}

func matchesAccept(accept, supported ContentType) bool {
	if accept == acceptAll {
		return true
	}
	return accept == supported
}

func NewJSONYAMLContentTypeRecognizer() ContentTypeRecognizer {
	return jsonYAMLContentTypeRecognizer{}
}

type jsonYAMLContentTypeRecognizer struct {
}

var defaultExtMap = map[string]ContentType{
	".json": ContentTypeJSON,
	".yml":  ContentTypeYAML,
	".yaml": ContentTypeYAML,
}

func (jsonYAMLContentTypeRecognizer) FromContentMetadata(m Metadata) (ContentType, bool) {
	loc, ok := metadata.GetString(m, metadata.XContentLocationKey)
	if !ok {
		return "", false
	}
	ext := filepath.Ext(loc)
	ct, ok := defaultExtMap[ext]
	if !ok {
		return "", false
	}
	return ct, true
}

func (jsonYAMLContentTypeRecognizer) FromPeekBytes(peek []byte) (ContentType, bool) {
	// Check if this is JSON or YAML
	if yamlutil.IsJSONBuffer(peek) {
		return ContentTypeJSON, true
	} else if isYAML(peek) {
		return ContentTypeYAML, true
	}
	return "", false
}

func (jsonYAMLContentTypeRecognizer) SupportedContentTypes() ContentTypes {
	return []ContentType{ContentTypeJSON, ContentTypeYAML}
}

func isYAML(peek []byte) bool {
	line, err := getLine(peek)
	if err != nil {
		return false
	}

	o := map[string]interface{}{}
	err = yaml.Unmarshal(line, &o)
	return err == nil
}

func getLine(peek []byte) ([]byte, error) {
	s := bufio.NewScanner(bytes.NewReader(peek))
	// TODO: Support very long lines? (over 65k bytes?) Probably not
	for s.Scan() {
		t := bytes.TrimSpace(s.Bytes())
		// TODO: Ignore comments
		if len(t) == 0 || bytes.Equal(t, []byte("---")) {
			continue
		}
		return t, nil
	}
	// Return a possible scanning error
	if err := s.Err(); err != nil {
		return nil, err
	}
	return nil, errors.New("couldn't find non-empty line in scanner")
}
