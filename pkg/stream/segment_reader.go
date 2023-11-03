package stream

import (
	"context"
	"io"

	"github.com/weaveworks/libgitops/pkg/tracing"
	"go.opentelemetry.io/otel/trace"
)

type segmentReader struct {
	MetadataContainer
	read  *readSegmentContextLockImpl
	close *closeContextLockImpl
}

func (r *segmentReader) WithContext(ctx context.Context) ClosableRawSegmentReader {
	return closableRawSegmentReader{&segmentReaderWithContext{r.read, ctx}, &closerWithContext{r.close, ctx}}
}

func (r *segmentReader) RawSegmentReader() RawSegmentReader { return r.read.r }
func (r *segmentReader) RawCloser() io.Closer               { return r.close.c }

type segmentReaderWithContext struct {
	read *readSegmentContextLockImpl
	ctx  context.Context
}

func (r *segmentReaderWithContext) Read() (content []byte, err error) {
	r.read.setContext(r.ctx)
	content, err = r.read.Read()
	r.read.clearContext()
	return
}

type readSegmentContextLockImpl struct {
	contextLockImpl
	r              RawSegmentReader
	metaGetter     MetadataContainer
	underlyingLock contextLock
}

func (r *readSegmentContextLockImpl) Read() (content []byte, err error) {
	ft := tracing.FromContext(r.ctx, r.r)
	err = ft.TraceFunc(r.ctx, "ReadSegment", func(ctx context.Context, span trace.Span) error {
		var tmperr error
		if r.underlyingLock != nil {
			r.underlyingLock.setContext(ctx)
		}
		content, tmperr = r.r.Read()
		if r.underlyingLock != nil {
			r.underlyingLock.clearContext()
		}
		span.SetAttributes(SpanAttrByteContent(content)...)
		return tmperr
	}, trace.WithAttributes(SpanAttrContentMetadata(r.metaGetter.ContentMetadata()))).RegisterCustom(SpanRegisterReadError)
	return
}

type closableRawSegmentReader struct {
	RawSegmentReader
	io.Closer
}
