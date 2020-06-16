package serializer

import (
	"fmt"
	"io"

	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	k8sserializer "k8s.io/apimachinery/pkg/runtime/serializer"
)

// Serializer is an interface providing high-level decoding/encoding functionality
// for types registered in a *runtime.Scheme
type Serializer interface {
	/*// DecodeInto takes byte content and a target object to serialize the data into
	DecodeInto(content []byte, obj runtime.Object) error
	// DecodeFileInto takes a file path and a target object to serialize the data into
	DecodeFileInto(filePath string, obj runtime.Object) error

	// Decode takes byte content and returns the target object
	Decode(content []byte, internal bool) (runtime.Object, error)
	// DecodeFile takes a file path and returns the target object
	DecodeFile(filePath string, internal bool) (runtime.Object, error)

	// EncodeYAML encodes the specified object for a specific version to YAML bytes
	EncodeYAML(obj runtime.Object) ([]byte, error)
	// EncodeJSON encodes the specified object for a specific version to pretty JSON bytes
	EncodeJSON(obj runtime.Object) ([]byte, error)*/

	ByteDecoder(content []byte) Decoder
	StreamDecoder(r io.ReadCloser) Decoder
	FileDecoder(filePath string) Decoder

	JSONEncoder() Encoder
	YAMLEncoder() Encoder

	// DefaultInternal populates the given internal object with the preferred external version's defaults
	DefaultInternal(cfg runtime.Object) error

	// Scheme provides access to the underlying runtime.Scheme
	Scheme() *runtime.Scheme
}

type schemeAndCodec struct {
	scheme *runtime.Scheme
	codecs *k8sserializer.CodecFactory
}

type EncodingOptions struct {
	// Default: true
	Pretty bool
}

type Encoder interface {
	// Encode returns the objects in the specified encoded format. In case multiple objects are
	// provided, the encoder will specify behavior. For YAML, multiple documents will be written. For
	// JSON, this call will error. This encoder will choose the preferred external groupversion automatically.
	Encode(obj ...runtime.Object) ([]byte, error)

	// TODO: Maybe add this?
	// EncodeForGroupVersion(obj runtime.Object, gv schema.GroupVersion)

	// TODO: How to specify target streams?

	// WithOptions returns a new encoder with the specified options
	WithOptions(opts EncodingOptions) Encoder
}

type DecodingOptions struct {
	// Default: false
	Internal bool
	// Default: true
	Strict bool
	// Default: false
	Default bool
}

type Decoder interface {
	// Decode takes byte content and returns the target object
	// Errors if the populated document contains more than one YAML document
	Decode() (runtime.Object, error)
	// DecodeInto takes byte content and a target object to serialize the data into
	// Errors if the populated document contains more than one YAML document
	DecodeInto(obj runtime.Object) error

	// DecodeMultiple supports reading multiple YAML documents at once
	DecodeMultiple() ([]runtime.Object, error)

	// WithOptions returns a new decoder with the specified options
	WithOptions(opts DecodingOptions) Decoder
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
		// Construct a codec that uses the strict serializer, but also performs defaulting & conversion
		//decoder: codecs.CodecForVersions(nil, strictSerializer, nil, runtime.InternalGroupVersioner),
	}
}

// serializer implements the Serializer interface
type serializer struct {
	*schemeAndCodec
	//decoder runtime.Decoder
}

// Scheme provides access to the underlying runtime.Scheme
func (s *serializer) Scheme() *runtime.Scheme {
	return s.scheme
}

func (s *serializer) StreamDecoder(rc io.ReadCloser) Decoder {
	return newStreamDecoder(rc, s.schemeAndCodec, defaultDecodeOpts)
}

func (s *serializer) ByteDecoder(content []byte) Decoder {
	return newBytesDecoder(content, s.schemeAndCodec, defaultDecodeOpts)
}

func (s *serializer) FileDecoder(filePath string) Decoder {
	return newFileDecoder(filePath, s.schemeAndCodec, defaultDecodeOpts)
}

func (s *serializer) JSONEncoder() Encoder {
	return newEncoder(s.schemeAndCodec, runtime.ContentTypeJSON, defaultEncodingOpts)
}

func (s *serializer) YAMLEncoder() Encoder {
	return newEncoder(s.schemeAndCodec, runtime.ContentTypeYAML, defaultEncodingOpts)
}

// DecodeFileInto takes a file path and a target object to serialize the data into
/*func (s *serializer) DecodeFileInto(filePath string, obj runtime.Object) error {
	content, err := ioutil.ReadFile(filePath)
	if err != nil {
		return err
	}

	return s.DecodeInto(content, obj)
}

// DecodeInto takes byte content and a target object to serialize the data into
func (s *serializer) DecodeInto(content []byte, obj runtime.Object) error {
	return runtime.DecodeInto(s.decoder, content, obj)
}

// DecodeFile takes a file path and returns the target object
func (s *serializer) DecodeFile(filePath string, internal bool) (runtime.Object, error) {
	content, err := ioutil.ReadFile(filePath)
	if err != nil {
		return nil, err
	}

	return s.Decode(content, internal)
}

// Decode takes byte content and returns the target object
func (s *serializer) Decode(content []byte, internal bool) (runtime.Object, error) {
	obj, err := runtime.Decode(s.decoder, content)
	if err != nil {
		return nil, err
	}
	// Default the object
	s.scheme.Default(obj)

	// If we did not request an internal conversion, return quickly
	if !internal {
		return obj, nil
	}
	// Return the internal version of the object
	return s.scheme.ConvertToVersion(obj, runtime.InternalGroupVersioner)
}

// EncodeYAML encodes the specified object for a specific version to YAML bytes
func (s *serializer) EncodeYAML(obj runtime.Object) ([]byte, error) {
	return s.encode(obj, runtime.ContentTypeYAML, false)
}

// EncodeJSON encodes the specified object for a specific version to pretty JSON bytes
func (s *serializer) EncodeJSON(obj runtime.Object) ([]byte, error) {
	return s.encode(obj, runtime.ContentTypeJSON, true)
}*/

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
