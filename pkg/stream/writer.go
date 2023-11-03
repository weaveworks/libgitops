package stream

import (
	"context"
	"io"

	"github.com/weaveworks/libgitops/pkg/stream/metadata"
	"github.com/weaveworks/libgitops/pkg/tracing"
	"github.com/weaveworks/libgitops/pkg/util/compositeio"
	"go.opentelemetry.io/otel/trace"
)

func NewWriter(w io.Writer, opts ...metadata.HeaderOption) Writer {
	// If it already is a Writer, just return it
	ww, ok := w.(Writer)
	if ok {
		return ww
	}

	// Use the closer if available
	c, _ := w.(io.Closer)
	// Never close stdio
	if isStdio(w) {
		c = nil
	}
	mb := NewMetadata(opts...).ToContainer()

	return &writer{
		MetadataContainer: mb,
		write: &writeContextLockImpl{
			w:          w,
			metaGetter: mb,
			// underlyingLock is nil
		},
		close: &closeContextLockImpl{
			c:          c,
			metaGetter: mb,
			// underlyingLock is nil
		},
	}
}

type writer struct {
	MetadataContainer
	write *writeContextLockImpl
	close *closeContextLockImpl
}

func (w *writer) WithContext(ctx context.Context) io.WriteCloser {
	return compositeio.WriteCloser(&writerWithContext{w.write, ctx}, &closerWithContext{w.close, ctx})
}
func (w *writer) RawWriter() io.Writer { return w.write.w }
func (w *writer) RawCloser() io.Closer { return w.close.c }

func (w *writer) Wrap(wrapFn WrapWriterFunc) Writer {
	newWriter := wrapFn(compositeio.WriteCloser(w.write, w.close))
	if newWriter == nil {
		panic("newWriter must not be nil")
	}
	// If an io.Closer is not returned, close this
	// Reader's stream instead. Importantly enough,
	// a trace will be registered for both this
	// Reader, and the returned one.
	newCloser, ok := newWriter.(io.Closer)
	if !ok {
		newCloser = w.close
	}

	mb := w.ContentMetadata().Clone().ToContainer()

	return &writer{
		MetadataContainer: mb,
		write: &writeContextLockImpl{
			w:              newWriter,
			metaGetter:     mb,
			underlyingLock: w.write,
		},
		close: &closeContextLockImpl{
			c:              newCloser,
			metaGetter:     mb,
			underlyingLock: w.close,
		},
	}
}

type writerWithContext struct {
	write *writeContextLockImpl
	ctx   context.Context
}

func (w *writerWithContext) Write(p []byte) (n int, err error) {
	w.write.setContext(w.ctx)
	n, err = w.write.Write(p)
	w.write.clearContext()
	return
}

type writeContextLockImpl struct {
	contextLockImpl
	w              io.Writer
	metaGetter     MetadataContainer
	underlyingLock contextLock
}

func (r *writeContextLockImpl) Write(p []byte) (n int, err error) {
	ft := tracing.FromContext(r.ctx, r.w)
	err = ft.TraceFunc(r.ctx, "Write", func(ctx context.Context, span trace.Span) error {
		var tmperr error
		if r.underlyingLock != nil {
			r.underlyingLock.setContext(ctx)
		}
		n, tmperr = r.w.Write(p)
		if r.underlyingLock != nil {
			r.underlyingLock.clearContext()
		}
		// Register metadata in the span
		span.SetAttributes(SpanAttrByteContentCap(p[:n], len(p))...)
		return tmperr
	}, trace.WithAttributes(SpanAttrContentMetadata(r.metaGetter.ContentMetadata()))).Register()
	return
}
