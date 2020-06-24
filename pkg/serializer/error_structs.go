package serializer

var _ ReadCloser = &errReadCloser{}

type errReadCloser struct {
	err error
}

func (rc *errReadCloser) Read(p []byte) (n int, err error) {
	err = rc.err
	return
}

func (rc *errReadCloser) Close() error {
	return nil
}

var _ FrameReader = &errFrameReader{}

type errFrameReader struct {
	err         error
	contentType ContentType
}

func (fr *errFrameReader) ReadFrame() ([]byte, error) {
	return nil, fr.err
}

func (fr *errFrameReader) ContentType() ContentType {
	return fr.contentType
}

// Close implements io.Closer and closes the underlying ReadCloser
func (fr *errFrameReader) Close() error {
	return nil
}

var _ FrameWriter = &errFrameWriter{}

type errFrameWriter struct {
	err         error
	contentType ContentType
}

func (fw *errFrameWriter) Write(_ []byte) (n int, err error) {
	err = fw.err
	return
}

func (fw *errFrameWriter) ContentType() ContentType {
	return fw.contentType
}
