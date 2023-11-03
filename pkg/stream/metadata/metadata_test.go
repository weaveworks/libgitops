package metadata

import (
	"net/textproto"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestGetMediaTypes(t *testing.T) {
	tests := []struct {
		name           string
		opts           []HeaderOption
		key            string
		wantMediaTypes []string
		wantErr        error
	}{
		{
			name: "multiple keys, and values in one key",
			opts: []HeaderOption{
				WithAccept("application/yaml", "application/xml"),
				WithAccept("application/json"),
				WithAccept("text/html, application/xhtml+xml, application/xml;q=0.9, image/webp, */*;q=0.8"),
			},
			key: AcceptKey,
			wantMediaTypes: []string{
				"application/yaml",
				"application/xml",
				"application/json",
				"text/html",
				"application/xhtml+xml",
				"application/xml",
				"image/webp",
				"*/*",
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := textproto.MIMEHeader{}
			for _, opt := range tt.opts {
				opt.ApplyToHeader(h)
			}
			gotMediaTypes, err := GetMediaTypes(h, tt.key)
			assert.Equal(t, tt.wantMediaTypes, gotMediaTypes)
			assert.ErrorIs(t, err, tt.wantErr)
		})
	}
}
