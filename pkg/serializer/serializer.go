package serializer

import (
	"bytes"
	"fmt"
	"io"
	"os"

	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	k8sserializer "k8s.io/apimachinery/pkg/runtime/serializer"
)

func newReaderWithClose(r io.Reader) ReadCloser {
	return &readerWithClose{r}
}

type readerWithClose struct {
	io.Reader
}

func (readerWithClose) Close() error {
	return nil
}

type ReadCloser io.ReadCloser

func FromFile(filePath string) ReadCloser {
	f, err := os.Open(filePath)
	if err != nil {
		return &errReadCloser{err}
	}
	return f
}

func FromBytes(content []byte) ReadCloser {
	return newReaderWithClose(bytes.NewReader(content))
}

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

type ContentType string

const (
	ContentTypeJSON ContentType = ContentType(runtime.ContentTypeJSON)
	ContentTypeYAML ContentType = ContentType(runtime.ContentTypeYAML)
)

// Serializer is an interface providing high-level decoding/encoding functionality
// for types registered in a *runtime.Scheme
type Serializer interface {
	// Decoder returns a decoder with the given options and reader. You may use helper functions
	// FromBytes and FromFile to decode from a file or byte slice. The decoder should be closed
	// after use.
	Decoder(rc ReadCloser, optsFn ...DecodingOptionsFunc) Decoder

	// Encoder returns an encoder for the specified content type and options. Encode functions return bytes, but
	// there's also the option to write all encoded content during the lifetime of the encoder to a writer using
	// WithEncodeWriter(io.Writer) in the options.
	Encoder(contentType ContentType, optsFn ...EncodingOptionsFunc) Encoder

	// DefaultInternal populates the given internal object with the preferred external version's defaults
	// TODO: Make Defaulter() interface
	DefaultInternal(cfg runtime.Object) error

	// Scheme provides access to the underlying runtime.Scheme
	Scheme() *runtime.Scheme
}

type schemeAndCodec struct {
	scheme *runtime.Scheme
	codecs *k8sserializer.CodecFactory
}

type Encoder interface {
	// Encode returns the objects in the specified encoded format. In case multiple objects are
	// provided, the encoder will specify behavior. For YAML, multiple documents will be written. For
	// JSON, this call will error. This encoder will choose the preferred external groupversion automatically.
	Encode(obj ...runtime.Object) ([]byte, error)

	// TODO: Maybe add this?
	// EncodeForGroupVersion(obj runtime.Object, gv schema.GroupVersion) ([]byte, error)
}

type Decoder interface {
	// Decode returns the decoded object from the next document in the stream.
	// If there are multiple documents in the underlying stream, this call will read one
	// 	document and return it. Decode might be invoked for getting new documents until it
	// 	returns io.EOF. When io.EOF is reached in a call, the stream is automatically closed.
	// If opts.Default is true, the decoded object will be defaulted.
	// If opts.Strict is true, the YAML/JSON will be parsed in strict mode, returning a specific error (TODO)
	// 	if the input contains duplicate or unknown fields or formatting errors
	// If opts.Internal is true, the decoded external object will be converted into its internal representation.
	// 	Otherwise, the decoded object will be left in the external representation.
	// opts.DecodeListElements is not applicable in this call. If the underlying data contains a v1.List,
	// 	decoding will be successfully performed and a v1.List is returned.
	// TODO: Mention UnrecognizedGroupError and UnrecognizedVersionError
	Decode() (runtime.Object, error)
	// DecodeInto decodes the next document in the stream into obj if the types are matching.
	// If there are multiple documents in the underlying stream, this call will read one
	// 	document and return it. Decode might be invoked for getting new documents until it
	// 	returns io.EOF. When io.EOF is reached in a call, the stream is automatically closed.
	// If opts.Default is true, the decoded object will be defaulted.
	// If opts.Strict is true, the YAML/JSON will be parsed in strict mode, returning a specific error (TODO)
	// 	if the input contains duplicate or unknown fields or formatting errors
	// opts.DecodeListElements is not applicable in this call. If a v1.List is given as obj, and the
	// 	underlying data contains a v1.List, decoding will be successfully performed.
	// opts.Internal is not applicable in this call.
	DecodeInto(obj runtime.Object) error

	// DecodeAll returns the decoded objects from all documents in the stream. The underlying
	// stream is automatically closed on io.EOF. io.EOF is never returned from this function.
	// If opts.Default is true, the decoded objects will be defaulted.
	// If opts.Strict is true, the YAML/JSON will be parsed in strict mode, returning a specific error (TODO)
	// 	if the input contains duplicate or unknown fields or formatting errors
	// If opts.Internal is true, the decoded external object will be converted into their internal representation.
	// 	Otherwise, the decoded objects will be left in their external representation.
	// If opts.DecodeListElements is true and the underlying data contains a v1.List,
	// 	the items of the list will be traversed and decoded into their respective types, which are
	// 	added into the returning slice. The v1.List will in this case not be returned.
	DecodeAll() ([]runtime.Object, error)

	// Close implements io.Closer. If close is called, the underlying stream is also closed. After
	// Close() has been called, all future Decode operations will return an error.
	Close() error
}

// NewSerializer constructs a new serializer based on a scheme, and optionally a codecfactory
func NewSerializer(scheme *runtime.Scheme, codecs *k8sserializer.CodecFactory) Serializer {
	if scheme == nil {
		panic("scheme must not be nil")
	}

	if codecs == nil {
		codecs = &k8sserializer.CodecFactory{}
		*codecs = k8sserializer.NewCodecFactory(scheme)
	}

	return &serializer{
		schemeAndCodec: &schemeAndCodec{
			scheme: scheme,
			codecs: codecs,
		},
	}
}

// serializer implements the Serializer interface
type serializer struct {
	*schemeAndCodec
}

// Scheme provides access to the underlying runtime.Scheme
func (s *serializer) Scheme() *runtime.Scheme {
	return s.scheme
}

func (s *serializer) Decoder(rc ReadCloser, optFns ...DecodingOptionsFunc) Decoder {
	opts := newDecodeOpts(optFns...)
	return newDecoder(s.schemeAndCodec, rc, *opts)
}

func (s *serializer) Encoder(contentType ContentType, optFns ...EncodingOptionsFunc) Encoder {
	opts := newEncodeOpts(optFns...)
	return newEncoder(s.schemeAndCodec, contentType, *opts)
}

// DefaultInternal populates the given internal object with the preferred external version's defaults
func (s *serializer) DefaultInternal(cfg runtime.Object) error {
	gvk, err := s.externalGVKForObject(cfg)
	if err != nil {
		return err
	}
	external, err := s.scheme.New(*gvk)
	if err != nil {
		return nil
	}
	if err := s.scheme.Convert(cfg, external, nil); err != nil {
		return err
	}
	s.scheme.Default(external)
	return s.scheme.Convert(external, cfg, nil)
}

func (s *serializer) externalGVKForObject(cfg runtime.Object) (*schema.GroupVersionKind, error) {
	gvks, unversioned, err := s.scheme.ObjectKinds(cfg)
	if unversioned || err != nil || len(gvks) != 1 {
		return nil, fmt.Errorf("unversioned %t or err %v or invalid gvks %v", unversioned, err, gvks)
	}

	gvk := gvks[0]
	gvs := s.scheme.PrioritizedVersionsForGroup(gvk.Group)
	if len(gvs) < 1 {
		return nil, fmt.Errorf("expected some version to be registered for group %s", gvk.Group)
	}

	// Use the preferred (external) version
	gvk.Version = gvs[0].Version
	return &gvk, nil
}
