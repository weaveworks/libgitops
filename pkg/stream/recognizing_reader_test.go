package stream

import (
	"bufio"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

func Test_isYAML(t *testing.T) {
	tests := []struct {
		name string
		peek string
		want bool
	}{
		{
			name: "field mapping",
			peek: "foo: bar\n",
			want: true,
		},
		{
			name: "spaces and other empty documents",
			peek: `---


---
---
foo: bar`,
			want: true,
		},
		{
			name: "bool",
			peek: "foo: true",
			want: true,
		},
		{
			name: "int",
			peek: "foo: 5",
			want: true,
		},
		{
			name: "float",
			peek: "foo: 5.1",
			want: true,
		},
		{
			name: "float",
			peek: "foo: null",
			want: true,
		},
		{
			name: "beginning of struct",
			peek: "foo:",
			want: true,
		},
		{
			name: "scalar null",
			peek: `null`,
			want: true,
		},
		{
			name: "nothing",
		},
		{
			name: "line overflow",
			peek: strings.Repeat("a", bufio.MaxScanTokenSize) + ": true",
		},

		{
			name: "list element struct",
			peek: "- foo: bar",
		},
		{
			name: "list element string",
			peek: "- foo",
		},
		{
			name: "scalar string",
			peek: `foo`,
		},
		{
			name: "scalar int",
			peek: `5`,
		},
		{
			name: "scalar float",
			peek: `5.1`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, isYAML([]byte(tt.peek)), tt.want)
		})
	}
}
