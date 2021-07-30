package frame

import (
	"bytes"
	"context"
	"io"
	"io/fs"
	"io/ioutil"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/weaveworks/libgitops/pkg/stream"
	"github.com/weaveworks/libgitops/pkg/tracing"
	"github.com/weaveworks/libgitops/pkg/util/compositeio"
	"github.com/weaveworks/libgitops/pkg/util/limitedio"
)

type rawCloserExposer interface {
	RawCloser() io.Closer
}

func TestFromConstructors(t *testing.T) {
	yamlPath := filepath.Join(t.TempDir(), "foo.yaml")
	str := "foo: bar\n"
	byteContent := []byte(str)
	err := ioutil.WriteFile(yamlPath, byteContent, 0644)
	require.Nil(t, err)

	ctx := tracing.Context(true)
	// FromYAMLFile -- found
	got, err := FromYAMLFile(yamlPath).ReadFrame(ctx)
	assert.Nil(t, err)
	assert.Equal(t, str, string(got))
	// stream.FromFile -- already closed
	f := stream.FromFile(yamlPath)
	(f.(rawCloserExposer)).RawCloser().Close() // deliberately close the file before giving it to the reader
	got, err = NewYAMLReader(f).ReadFrame(ctx)
	assert.ErrorIs(t, err, fs.ErrClosed)
	assert.Empty(t, got)
	// FromYAMLFile -- not found
	got, err = FromYAMLFile(filepath.Join(t.TempDir(), "notexist.yaml")).ReadFrame(ctx)
	assert.ErrorIs(t, err, fs.ErrNotExist)
	assert.Empty(t, got)
	// FromYAMLBytes
	got, err = FromYAMLBytes(byteContent).ReadFrame(ctx)
	assert.Nil(t, err)
	assert.Equal(t, byteContent, got)
	// FromYAMLString
	got, err = FromYAMLString(str).ReadFrame(ctx)
	assert.Nil(t, err)
	assert.Equal(t, str, string(got))
	assert.Nil(t, tracing.ForceFlushGlobal(ctx, 0))
}

func TestToIoWriteCloser(t *testing.T) {
	var buf bytes.Buffer
	closeRec := &recordingCloser{}
	cw := stream.NewWriter(compositeio.WriteCloser(&buf, closeRec))
	w := NewYAMLWriter(cw, SingleOptions{MaxFrameSize: limitedio.Limit(testYAMLlen)})
	ctx := tracing.Context(true)
	iow := ToIoWriteCloser(ctx, w)

	byteContent := []byte(testYAML)
	n, err := iow.Write(byteContent)
	assert.Len(t, byteContent, n)
	assert.Nil(t, err)

	// Check closing forwarding
	assert.Nil(t, iow.Close())
	assert.Equal(t, 1, closeRec.count)

	// Try writing again
	overflowContent := []byte(testYAML + testYAML)
	n, err = iow.Write(overflowContent)
	assert.Equal(t, 0, n)
	assert.ErrorIs(t, err, &limitedio.ReadSizeOverflowError{})
	// Assume the writer has been closed only once
	assert.Equal(t, 1, closeRec.count)
	assert.Equal(t, buf.String(), yamlSep+string(byteContent))

	assert.Nil(t, tracing.ForceFlushGlobal(context.Background(), 0))
}

func TestListFromReader(t *testing.T) {
	ctx := tracing.Context(true)
	// Happy case
	fr, err := ListFromReader(ctx, FromYAMLString(messyYAML))
	assert.Equal(t, List{[]byte(testYAML), []byte(testYAML)}, fr)
	assert.Nil(t, err)

	// Non-happy case
	r := NewJSONReader(stream.FromString(testJSON2), SingleOptions{MaxFrameSize: limitedio.Limit(testJSONlen - 1)})
	fr, err = ListFromReader(ctx, r)
	assert.Len(t, fr, 0)
	assert.ErrorIs(t, err, &limitedio.ReadSizeOverflowError{})
	assert.Nil(t, tracing.ForceFlushGlobal(ctx, 0))
}

func TestList_WriteTo(t *testing.T) {
	var buf bytes.Buffer
	// TODO: Automatically get the name of the writer passed in, to avoid having to name
	// everything. i.e. stream.NewWriterName(string, io.Writer)
	cw := stream.NewWriter(&buf)
	w := NewYAMLWriter(cw)
	ctx := context.Background()
	// Happy case
	err := ListFromBytes([]byte(testYAML), []byte(testYAML)).WriteTo(ctx, w)
	assert.Equal(t, buf.String(), yamlSep+testYAML+yamlSep+testYAML)
	assert.Nil(t, err)

	// Non-happy case
	buf.Reset()
	w = NewJSONWriter(cw, SingleOptions{MaxFrameSize: limitedio.Limit(testJSONlen)})
	err = ListFromBytes([]byte(testJSON), []byte(testJSON2)).WriteTo(ctx, w)
	assert.Equal(t, buf.String(), testJSON)
	assert.ErrorIs(t, err, &limitedio.ReadSizeOverflowError{})
}
