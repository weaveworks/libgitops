package content

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func Test_isStdio(t *testing.T) {
	tmp := t.TempDir()
	f, err := os.Create(filepath.Join(tmp, "foo.txt"))
	require.Nil(t, err)
	defer f.Close()
	tests := []struct {
		name string
		in   interface{}
		want bool
	}{
		{
			name: "os.Stdin",
			in:   os.Stdin,
			want: true,
		},
		{
			name: "os.Stdout",
			in:   os.Stdout,
			want: true,
		},
		{
			name: "os.Stderr",
			in:   os.Stderr,
			want: true,
		},
		{
			name: "*bytes.Buffer",
			in:   bytes.NewBufferString("FooBar"),
		},
		{
			name: "*strings.Reader",
			in:   strings.NewReader("FooBar"),
		},
		{
			name: "*strings.Reader",
			in:   strings.NewReader("FooBar"),
		},
		{
			name: "*os.File",
			in:   f,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := isStdio(tt.in)
			assert.Equal(t, got, tt.want)
		})
	}
}
