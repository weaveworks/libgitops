package serializer

import (
	"fmt"

	"github.com/weaveworks/libgitops/pkg/util"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/runtime/serializer/json"
)

type EncodingOptions struct {
	// Use pretty printing when writing to the output. (Default: true)
	Pretty *bool
}

type EncodingOptionsFunc func(*EncodingOptions)

func WithPrettyEncode(pretty bool) EncodingOptionsFunc {
	return func(opts *EncodingOptions) {
		opts.Pretty = &pretty
	}
}

func WithEncodingOptions(newOpts EncodingOptions) EncodingOptionsFunc {
	return func(opts *EncodingOptions) {
		*opts = newOpts
	}
}

func defaultEncodeOpts() *EncodingOptions {
	return &EncodingOptions{
		Pretty: util.BoolPtr(true),
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

	encoders map[ContentType]runtime.Serializer
	opts     EncodingOptions
}

func newEncoder(schemeAndCodec *schemeAndCodec, opts EncodingOptions) Encoder {
	return &encoder{
		schemeAndCodec,
		map[ContentType]runtime.Serializer{
			ContentTypeYAML: json.NewSerializerWithOptions(
				json.DefaultMetaFactory, schemeAndCodec.scheme, schemeAndCodec.scheme,
				json.SerializerOptions{Yaml: true, Pretty: *opts.Pretty, Strict: false},
			),
			ContentTypeJSON: json.NewSerializerWithOptions(
				json.DefaultMetaFactory, schemeAndCodec.scheme, schemeAndCodec.scheme,
				json.SerializerOptions{Yaml: false, Pretty: *opts.Pretty, Strict: false},
			),
		},
		opts,
	}
}

func (e *encoder) Encode(fw FrameWriter, objs ...runtime.Object) error {
	for _, obj := range objs {
		// Get the kind for the given object
		gvk, err := gvkForObject(e.scheme, obj)
		if err != nil {
			return err
		}

		// If the object is internal, convert it to the preferred external one
		fmt.Printf("GVK before: %s\n", gvk)
		if gvk.Version == runtime.APIVersionInternal {
			gvk, err = externalGVKForObject(e.scheme, obj)
			if err != nil {
				return err
			}
		}
		fmt.Printf("GVK after: %s\n", gvk)

		// Encode it
		if err := e.EncodeForGroupVersion(fw, obj, gvk.GroupVersion()); err != nil {
			return err
		}
	}
	return nil
}

func (e *encoder) EncodeForGroupVersion(fw FrameWriter, obj runtime.Object, gv schema.GroupVersion) error {
	// Get the generic encoder for the right content type
	enc, ok := e.encoders[fw.ContentType()]
	if !ok {
		return ErrUnsupportedContentType
	}
	fmt.Printf("Foo %s %s\n", enc.Identifier(), gv)

	// Specialize the encoder for a specific gv and encode the object
	return e.codecs.EncoderForVersion(enc, gv).Encode(obj, fw)
}
