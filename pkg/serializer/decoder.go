package serializer

import (
	"bufio"
	"bytes"
	"fmt"
	"io"
	"io/ioutil"

	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/serializer/json"
	"k8s.io/apimachinery/pkg/runtime/serializer/versioning"
	"k8s.io/apimachinery/pkg/util/yaml"
)

func ReaderWithClose(r io.Reader) io.ReadCloser {
	return &readerWithClose{r}
}

type readerWithClose struct {
	io.Reader
}

func (readerWithClose) Close() error {
	return nil
}

var defaultDecodeOpts = DecodingOptions{
	Internal: false,
	Strict:   true,
	Default:  false,
}

type streamDecoder struct {
	*schemeAndCodec
	rc         io.ReadCloser // this is the underlying reader of YAMLReader
	yamlReader *yaml.YAMLReader
	decoder    runtime.Decoder //serializer *json.Serializer
	opts       DecodingOptions
}

// Decode takes byte content and returns the target object
// Errors if the populated document contains more than one YAML document
func (d *streamDecoder) Decode() (runtime.Object, error) {
	objs, err := d.decodeMultiple(true)
	if err != nil {
		return nil, err
	}
	return objs[0], nil
}

// DecodeInto takes byte content and a target object to serialize the data into
// Errors if the populated document contains more than one YAML document. Should they?
func (d *streamDecoder) DecodeInto(obj runtime.Object) error {
	doc, err := d.readDoc()
	if err != nil {
		return err
	}
	return runtime.DecodeInto(d.decoder, doc, obj)
	// TODO: Fix erroring on more than one YAML document
}

// DecodeMultiple supports reading multiple YAML documents at once
func (d *streamDecoder) DecodeMultiple() ([]runtime.Object, error) {
	return d.decodeMultiple(false)
}

// TODO: Decode lists, too
func (d *streamDecoder) decodeMultiple(onlyOne bool) ([]runtime.Object, error) {
	//decoder := NewYAMLDecoder(d.rc, *d.codecs)
	//defer decoder.Close()

	/*// Return a structured error if the group was registered with the scheme but the version was unrecognized
	if gvk != nil && err != nil {
		if cs.scheme.IsGroupRegistered(gvk.Group) && !cs.scheme.IsVersionRegistered(gvk.GroupVersion()) {
			return NewUnrecognizedVersionError("please specify a correct API version", *gvk, data)
		}
	}*/

	objs := []runtime.Object{}
	for {
		doc, err := d.readDoc()
		if err == io.EOF {
			break
		} else if err != nil {
			return nil, err
		} else if onlyOne && len(objs) == 1 {
			// if we only expected one object, but got multiple, error
			return nil, fmt.Errorf("got more than one object, but expected one")
		}

		// Use our own "special" decoder
		obj, _, err := d.decoder.Decode(doc, nil, nil)
		if err != nil {
			return nil, err
		}

		if d.opts.Default {
			// Default the object
			d.scheme.Default(obj)
		}
		if d.opts.Internal {
			// Return the internal version of the object
			obj, err = d.scheme.ConvertToVersion(obj, runtime.InternalGroupVersioner)
			if err != nil {
				return nil, err // TODO: better here
			}
		}

		objs = append(objs, obj)
	}
	return objs, nil
}

func (d *streamDecoder) readDoc() ([]byte, error) {
	for {
		doc, err := d.yamlReader.Read()
		if err != nil {
			return nil, err
		}

		//  Skip over empty documents, i.e. a leading `---`
		if len(bytes.TrimSpace(doc)) == 0 {
			continue
		}

		// Return the YAML document
		return doc, nil
	}
}

// WithOptions returns a new Decoder with new options, preserving the same data & scheme
// The options are not defaulted, but used as-is. This call MUST happen before any Decode call
func (d *streamDecoder) WithOptions(opts DecodingOptions) Decoder {
	// TODO: Return err-decoder if we've already called any Decode call?
	return newStreamDecoder(d.rc, d.schemeAndCodec, opts)
}

func newStreamDecoder(rc io.ReadCloser, schemeAndCodec *schemeAndCodec, opts DecodingOptions) Decoder {
	// The YAML reader supports reading multiple YAML documents
	yamlReader := yaml.NewYAMLReader(bufio.NewReader(rc))

	// Allow both YAML and JSON inputs (JSON is a subset of YAML), and deserialize in strict mode
	s := json.NewSerializerWithOptions(json.DefaultMetaFactory, schemeAndCodec.scheme, schemeAndCodec.scheme, json.SerializerOptions{
		Yaml:   true,
		Strict: opts.Strict,
	})
	// Construct a codec that uses the strict serializer, but also performs defaulting & conversion
	//decoder := recognizer.NewDecoder(s) // schemeAndCodec.codecs.CodecForVersions(nil, s, nil, runtime.InternalGroupVersioner)
	// decoder := schemeAndCodec.codecs.UniversalDeserializer()
	decoder := newConversionCodecForScheme(schemeAndCodec.scheme, nil, s, nil, runtime.InternalGroupVersioner, opts.Default)

	return &streamDecoder{schemeAndCodec, rc, yamlReader, decoder, defaultDecodeOpts}
}

// newConversionCodecForScheme is a convenience method for callers that are using a scheme.
func newConversionCodecForScheme(
	scheme *runtime.Scheme,
	encoder runtime.Encoder,
	decoder runtime.Decoder,
	encodeVersion runtime.GroupVersioner,
	decodeVersion runtime.GroupVersioner,
	performDefaulting bool,
) runtime.Codec {
	var defaulter runtime.ObjectDefaulter
	if performDefaulting {
		defaulter = scheme
	}
	return versioning.NewCodec(encoder, decoder, runtime.UnsafeObjectConvertor(scheme), scheme, scheme, defaulter, encodeVersion, decodeVersion, scheme.Name())
}

func newBytesDecoder(b []byte, schemeAndCodec *schemeAndCodec, opts DecodingOptions) Decoder {
	return newStreamDecoder(ReaderWithClose(bytes.NewReader(b)), schemeAndCodec, opts)
}

func newFileDecoder(filePath string, schemeAndCodec *schemeAndCodec, opts DecodingOptions) Decoder {
	b, err := ioutil.ReadFile(filePath)
	if err != nil {
		return &errDecoder{err}
	}

	return newBytesDecoder(b, schemeAndCodec, opts)
}

type errDecoder struct {
	err error
}

// Decode takes byte content and returns the target object
// Errors if the populated document contains more than one YAML document
// The errDecoder always returns nil and the stored error
func (d *errDecoder) Decode() (runtime.Object, error) {
	return nil, d.err
}

// DecodeInto takes byte content and a target object to serialize the data into
// Errors if the populated document contains more than one YAML document
// The errDecoder always returns nil and the stored error
func (d *errDecoder) DecodeInto(obj runtime.Object) error {
	return d.err
}

// DecodeMultiple supports reading multiple YAML documents at once
// The errDecoder always returns nil and the stored error
func (d *errDecoder) DecodeMultiple() ([]runtime.Object, error) {
	return nil, d.err
}

// WithOptions sets the options for the decoder with the specified options, and returns itself
// This call modifies the internal state. The options are not defaulted, but used as-is
// The errDecoder always returns nil and the stored error
func (d *errDecoder) WithOptions(_ DecodingOptions) Decoder {
	return d
}
