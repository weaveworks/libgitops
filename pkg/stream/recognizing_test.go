package stream

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/weaveworks/libgitops/pkg/stream/metadata"
)

func Test_negotiateAccept(t *testing.T) {
	tests := []struct {
		name      string
		accepts   []string
		supported []ContentType
		want      ContentType
		wantOk    bool
	}{
		{
			name: "accepts has higher priority than supported",
			// application/bar is not supported, but the second highest priority does
			accepts:   []string{"application/bar", "application/json", "application/yaml"},
			supported: []ContentType{"application/foo", "application/yaml", "application/json"},
			want:      "application/json",
			wantOk:    true,
		},
		{
			name:      "no accepts should give empty result",
			supported: []ContentType{"application/foo", "application/yaml", "application/json"},
		},
		{
			name:    "no supported should give empty result",
			accepts: []string{"application/bar", "application/json", "application/yaml"},
		},
		{
			name:      "invalid accept should give empty result",
			accepts:   []string{"///;;app/bar", "application/json", "application/yaml"},
			supported: []ContentType{"application/foo", "application/yaml", "application/json"},
		},
		{
			name:      "ignore extra parameters, e.g. q=0.8",
			accepts:   []string{"application/bar", "application/json;q=0.8", "application/yaml"},
			supported: []ContentType{"application/foo", "application/yaml", "application/json"},
			want:      "application/json",
			wantOk:    true,
		},
		{
			name:      "allow comma separation",
			accepts:   []string{"application/bar, application/json;q=0.8", "application/yaml"},
			supported: []ContentType{"application/foo", "application/yaml", "application/json"},
			want:      "application/json",
			wantOk:    true,
		},
		{
			name:      "accept all; choose the preferred one",
			accepts:   []string{"application/bar, */*;q=0.7", "application/yaml"},
			supported: []ContentType{"application/foo", "application/yaml", "application/json"},
			want:      "application/foo",
			wantOk:    true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := NewMetadata(metadata.WithAccept(tt.accepts...))
			got, gotOk := negotiateAccept(m, tt.supported)
			assert.Equal(t, tt.want, got)
			assert.Equal(t, tt.wantOk, gotOk)
		})
	}
}
