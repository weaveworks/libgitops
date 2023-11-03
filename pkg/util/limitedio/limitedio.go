package limitedio

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"strconv"

	"github.com/weaveworks/libgitops/pkg/util/structerr"
)

// DefaultMaxReadSize is 3 MB, which matches the default behavior of Kubernetes.
// (The API server only accepts request bodies of 3MB by default.)
const DefaultMaxReadSize Limit = 3 * 1024 * 1024
const Infinite Limit = -1

type Limit int64

func (l Limit) String() string {
	if l <= 0 {
		return "infinite"
	}
	return strconv.FormatInt(int64(l), 10)
}
func (l Limit) Int64() int64 { return int64(l) }
func (l Limit) Int() (int, error) {
	i := int(l)
	if int64(i) != int64(l) {
		return 0, errors.New("the limit overflows int")
	}
	return i, nil
}

func (l Limit) IsLessThan(len int64) bool {
	// l <= 0 means "l is infinite" => limit is larger than len => not less than len
	if l <= 0 {
		return false
	}
	return l.Int64() < len
}

func (l Limit) IsLessThanOrEqual(len int64) bool {
	// l <= 0 means "l is infinite" => limit is larger than len => not less than len
	if l <= 0 {
		return false
	}
	return l.Int64() <= len
}

// ErrReadSizeOverflow returns a new *ReadSizeOverflowError
func ErrReadSizeOverflow(maxReadSize Limit) *ReadSizeOverflowError {
	return &ReadSizeOverflowError{MaxReadSize: maxReadSize}
}

// Enforce all struct errors implementing structerr.StructError
var _ structerr.StructError = &ReadSizeOverflowError{}

// ReadSizeOverflowError describes that a read or write has grown larger than
// allowed. It is up to the implementer to describe what a "frame" in this
// context is. This error is e.g. returned from the NewReader implementation.
// If MaxReadSize is non-zero, it is included in the error text.
//
// This error can be checked for equality using errors.Is(err, &ReadSizeOverflowError{})
type ReadSizeOverflowError struct {
	// +optional
	MaxReadSize Limit
}

func (e *ReadSizeOverflowError) Error() string {
	msg := "frame was larger than maximum allowed size"
	if e.MaxReadSize != 0 {
		msg = fmt.Sprintf("%s %d bytes", msg, e.MaxReadSize)
	}
	return msg
}

func (e *ReadSizeOverflowError) Is(target error) bool {
	_, ok := target.(*ReadSizeOverflowError)
	return ok
}

// Reader is a specialized io.Reader helper type, which allows detecting when
// a read grows larger than the allowed maxReadSize, returning a ErrReadSizeOverflow in that case.
//
// Internally there's a byte counter registering how many bytes have been read using the io.Reader
// across all Read calls since the last ResetCounter reset, which resets the byte counter to 0. This
// means that if you have successfully read one frame within bounds of maxReadSize, and want to
// re-use the underlying io.Reader for the next frame, you shall run ResetCounter to start again.
//
// maxReadSize is specified when constructing an Reader, and defaults to DefaultMaxReadSize
// if left as the empty value 0.
// If maxReadSize is negative, the reader transparently forwards all calls without any restrictions.
//
// Note: The Reader implementation is not thread-safe, that is for higher-level interfaces
// to implement and ensure.
type Reader interface {
	// The byte count returned across consecutive Read(p) calls are at maximum maxReadSize, until reset
	// by ResetCounter.
	io.Reader
	// ResetCounter resets the byte counter counting how many bytes have been read using Read(p)
	ResetCounter()
}

// NewReader makes a new Reader implementation.
func NewReader(r io.Reader, maxReadSize Limit) Reader {
	// Default maxReadSize if unset.
	if maxReadSize == 0 {
		maxReadSize = DefaultMaxReadSize
	}

	return &ioLimitedReader{
		reader:      r,
		buf:         new(bytes.Buffer),
		maxReadSize: maxReadSize,
	}
}

type ioLimitedReader struct {
	reader      io.Reader
	buf         *bytes.Buffer
	maxReadSize Limit
	byteCounter int64
}

func (l *ioLimitedReader) Read(b []byte) (int, error) {
	// If l.maxReadSize is negative, put no restrictions on the read
	maxReadSize := l.maxReadSize.Int64()
	if maxReadSize < 0 {
		return l.reader.Read(b)
	}
	// If we've already read more than we're allowed to, return an overflow error
	if l.byteCounter > maxReadSize {
		// Keep returning this error as long as relevant
		return 0, ErrReadSizeOverflow(l.maxReadSize)

	} else if l.byteCounter == maxReadSize {
		// At this point we're not sure if the frame actually stops here or not
		// To figure that out; read one more byte into tmp
		tmp := make([]byte, 1)
		tmpn, err := l.reader.Read(tmp)

		// Write the read byte into the persistent buffer, for later use when l.byteCounter < l.maxReadSize
		_, _ = l.buf.Write(tmp[:tmpn])
		// Increase the byteCounter, as bytes written to buf counts as "read"
		l.byteCounter += int64(tmpn)

		// If no bytes were read; it's ok as we didn't exceed the limit. Return
		// the error; often nil or io.EOF in this case.
		if tmpn == 0 {
			return 0, err
		}
		// Return that the frame overflowed now, as were able to read the byte (tmpn must be 1)
		return 0, ErrReadSizeOverflow(l.maxReadSize)
	} // else l.byteCounter < l.maxReadSize

	// We can at maximum read bytesLeft bytes more, shrink b accordingly if b is larger than the
	// maximum allowed amount to read.
	bytesLeft := maxReadSize - l.byteCounter
	if int64(len(b)) > bytesLeft {
		b = b[:bytesLeft]
	}

	// First, flush any bytes in the buffer. By convention, the writes to buf have already
	// increased byteCounter, so no need to do that now. No need to check the error as buf
	// only returns io.EOF, and that's not important, it's even expected in most cases.
	m, _ := l.buf.Read(b)
	// Move the b slice forward m bytes as the m first bytes of b have now been populated
	b = b[m:]

	// Read from the reader into the rest of b
	n, err := l.reader.Read(b)
	// Register how many bytes have been read now additionally
	l.byteCounter += int64(n)
	return n, err
}

func (r *ioLimitedReader) ResetCounter() { r.byteCounter = 0 }
