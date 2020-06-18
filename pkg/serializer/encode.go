package serializer

import (
	"bytes"
	"fmt"
	"io"

	"github.com/weaveworks/libgitops/pkg/util"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/runtime/serializer/json"
)

type EncodingOptions struct {
	// Use pretty printing when writing to the output. (Default: true)
	Pretty *bool

	// Where to write all encoder output during the encoder's lifetime.
	// TODO: Implement this
	Writer io.Writer
}

type EncodingOptionsFunc func(*EncodingOptions)

func WithPrettyEncode(pretty bool) EncodingOptionsFunc {
	return func(opts *EncodingOptions) {
		opts.Pretty = &pretty
	}
}

func WithEncodeWriter(w io.Writer) EncodingOptionsFunc {
	return func(opts *EncodingOptions) {
		opts.Writer = w
	}
}

func defaultEncodeOpts() *EncodingOptions {
	return &EncodingOptions{
		Pretty: util.BoolPtr(true),
		Writer: nil,
	}
}

func newEncodeOpts(fns ...EncodingOptionsFunc) *EncodingOptions {
	opts := defaultEncodeOpts()
	for _, fn := range fns {
		fn(opts)
	}
	return opts
}

type encoder struct {
	*schemeAndCodec

	serializer  runtime.Serializer
	framer      runtime.Framer
	contentType ContentType
	opts        EncodingOptions
}

func newEncoder(schemeAndCodec *schemeAndCodec, contentType ContentType, opts EncodingOptions) Encoder {
	var s runtime.Serializer
	var framer runtime.Framer
	switch contentType {
	case ContentTypeYAML:
		s = json.NewSerializerWithOptions(
			json.DefaultMetaFactory, schemeAndCodec.scheme, schemeAndCodec.scheme,
			json.SerializerOptions{Yaml: true, Pretty: *opts.Pretty, Strict: false},
		)
		framer = json.YAMLFramer
	case ContentTypeJSON:
		s = json.NewSerializerWithOptions(
			json.DefaultMetaFactory, schemeAndCodec.scheme, schemeAndCodec.scheme,
			json.SerializerOptions{Yaml: false, Pretty: *opts.Pretty, Strict: false},
		)
		framer = json.Framer
	default:
		return &errEncoder{fmt.Errorf("unable to locate encoder -- %q is not a supported media type", contentType)}
	}

	return &encoder{schemeAndCodec, s, framer, contentType, opts}
}

func (e *encoder) Encode(objs ...runtime.Object) ([]byte, error) {
	buf := new(bytes.Buffer)
	fw := e.framer.NewFrameWriter(buf)
	for _, obj := range objs {
		gvk, err := e.externalGVKForObject(obj)
		if err != nil {
			return nil, err
		}

		encoder := e.codecs.EncoderForVersion(e.serializer, gvk.GroupVersion())
		if err := encoder.Encode(obj, fw); err != nil {
			return nil, err
		}
	}
	return bytes.TrimPrefix(buf.Bytes(), []byte("---\n")), nil
}

// TODO: De-duplicate with serializer
func (e *encoder) externalGVKForObject(cfg runtime.Object) (*schema.GroupVersionKind, error) {
	gvks, unversioned, err := e.scheme.ObjectKinds(cfg)
	if unversioned || err != nil || len(gvks) != 1 {
		return nil, fmt.Errorf("unversioned %t or err %v or invalid gvks %v", unversioned, err, gvks)
	}

	gvk := gvks[0]
	gvs := e.scheme.PrioritizedVersionsForGroup(gvk.Group)
	if len(gvs) < 1 {
		return nil, fmt.Errorf("expected some version to be registered for group %s", gvk.Group)
	}

	// Use the preferred (external) version
	gvk.Version = gvs[0].Version
	return &gvk, nil
}

type errEncoder struct {
	err error
}

// ...
// The errEncoder always returns nil and the stored error
func (e *errEncoder) Encode(obj ...runtime.Object) ([]byte, error) {
	return nil, e.err
}

// WithOptions sets the options for the decoder with the specified options, and returns itself
// This call modifies the internal state. The options are not defaulted, but used as-is
// TODO
// The errEncoder always returns nil and the stored error
func (e *errEncoder) WithOptions(_ EncodingOptions) Encoder {
	return e
}
