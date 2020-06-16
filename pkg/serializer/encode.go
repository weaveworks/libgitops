package serializer

import (
	"bytes"
	"fmt"
	"io"

	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/runtime/serializer/json"
)

var defaultEncodingOpts = EncodingOptions{
	Pretty: true,
}

type encoder struct {
	*schemeAndCodec

	serializer runtime.Serializer
	framer     runtime.Framer
	mediaType  string
	opts       EncodingOptions
}

func newEncoder(schemeAndCodec *schemeAndCodec, mediaType string, opts EncodingOptions) Encoder {
	var s runtime.Serializer
	var framer runtime.Framer
	switch mediaType {
	case runtime.ContentTypeYAML:
		s = json.NewSerializerWithOptions(
			json.DefaultMetaFactory, schemeAndCodec.scheme, schemeAndCodec.scheme,
			json.SerializerOptions{Yaml: true, Pretty: opts.Pretty, Strict: false},
		)
		framer = json.YAMLFramer
	case runtime.ContentTypeJSON:
		s = json.NewSerializerWithOptions(
			json.DefaultMetaFactory, schemeAndCodec.scheme, schemeAndCodec.scheme,
			json.SerializerOptions{Yaml: false, Pretty: opts.Pretty, Strict: false},
		)
		framer = json.Framer
	default:
		return &errEncoder{fmt.Errorf("unable to locate encoder -- %q is not a supported media type", mediaType)}
	}

	return &encoder{schemeAndCodec, s, framer, mediaType, opts}
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

func (e *encoder) WithOptions(opts EncodingOptions) Encoder {
	return newEncoder(e.schemeAndCodec, e.mediaType, opts)
}

type jsonFramer struct {
	w io.Writer
}

func (f jsonFramer) Write(p []byte) (n int, err error) {
	if f.w == nil {
		err = fmt.Errorf("doesn't support writing more than one document at a time!")
		return
	}
	n, err = f.w.Write(p)
	f.w = nil
	return
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
