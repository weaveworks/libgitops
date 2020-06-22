package serializer

import "io"

// io.ReadCloser wrapper for io.Readers used when reading bytes from a passed slice
type readerWithClose struct {
	io.Reader
}

func newReaderWithClose(r io.Reader) io.ReadCloser {
	return &readerWithClose{r}
}

func (readerWithClose) Close() error {
	return nil
}

// Interface compliance verification
var _ io.ReadCloser = &readerWithClose{}

// io.ReadCloser used to forward the error from a failed source open
type errReadCloser struct {
	err error
}

func (rc *errReadCloser) Read([]byte) (int, error) {
	return 0, rc.err
}

func (rc *errReadCloser) Close() error {
	return nil
}

// Interface compliance verification
var _ io.ReadCloser = &errReadCloser{}
