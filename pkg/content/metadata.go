package content

import (
	"encoding/json"
	"net/textproto"
	"net/url"

	"github.com/weaveworks/libgitops/pkg/content/metadata"
)

// Metadata is the interface that's common to contentMetadataOptions and a wrapper
// around a HTTP request.
type Metadata interface {
	metadata.Header
	metadata.HeaderOption

	// Apply applies the given Options to itself and returns itself, without
	// any deep-copying.
	Apply(opts ...metadata.HeaderOption) Metadata
	// ContentLength retrieves the standard "Content-Length" header
	ContentLength() (int64, bool)
	// ContentType retrieves the standard "Content-Type" header
	ContentType() (ContentType, bool)
	// ContentLocation retrieves the custom "X-Content-Location" header
	ContentLocation() (*url.URL, bool)

	// Clone makes a deep copy of the Metadata
	// TODO: Do we need this anymore?
	Clone() Metadata

	ToContainer() MetadataContainer
}

var _ Metadata = contentMetadata{}

var _ json.Marshaler = contentMetadata{}

func (m contentMetadata) MarshalJSON() ([]byte, error) {
	return json.Marshal(m.MIMEHeader)
}

func (m contentMetadata) ApplyToHeader(target metadata.Header) {
	for k, vals := range m.MIMEHeader {
		for i, val := range vals {
			if i == 0 {
				target.Set(k, val)
			} else {
				target.Add(k, val)
			}
		}
	}
}

func (m contentMetadata) Apply(opts ...metadata.HeaderOption) Metadata {
	for _, opt := range opts {
		opt.ApplyToHeader(m)
	}
	return m
}

func (m contentMetadata) ContentLength() (int64, bool) {
	return metadata.GetInt64(m, metadata.ContentLengthKey)
}

func (m contentMetadata) ContentType() (ContentType, bool) {
	ct, ok := metadata.GetString(m, metadata.ContentTypeKey)
	return ContentType(ct), ok
}

func (m contentMetadata) ContentLocation() (*url.URL, bool) {
	return metadata.GetURL(m, metadata.XContentLocationKey)
}

func (m contentMetadata) ToContainer() MetadataContainer {
	return &metadataContainer{m}
}

func (m contentMetadata) Clone() Metadata {
	m2 := make(textproto.MIMEHeader, len(m.MIMEHeader))
	for k, v := range m.MIMEHeader {
		m2[k] = v
	}
	return contentMetadata{m2}
}

type MetadataContainer interface {
	// ContentMetadata
	ContentMetadata() Metadata
}

func NewMetadata(opts ...metadata.HeaderOption) Metadata {
	return contentMetadata{MIMEHeader: textproto.MIMEHeader{}}.Apply(opts...)
}

type contentMetadata struct {
	textproto.MIMEHeader
}

type metadataContainer struct{ m Metadata }

func (b *metadataContainer) ContentMetadata() Metadata { return b.m }
