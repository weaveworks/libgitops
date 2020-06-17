package serializer

import (
	"bufio"
	"bytes"
	"errors"
	"fmt"
	"io"
	"reflect"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/runtime/serializer/json"
	"k8s.io/apimachinery/pkg/runtime/serializer/versioning"
	yamlmeta "k8s.io/apimachinery/pkg/runtime/serializer/yaml"
	"k8s.io/apimachinery/pkg/util/yaml"
)

type DecodingOptions struct {
	// Default: false
	Internal *bool
	// Default: true
	Strict *bool
	// Default: false
	Default *bool
	// Default: true
	DecodeListElements *bool // TODO: How to make this able to preserve comments?
}

type DecodingOptionsFunc func(*DecodingOptions)

func WithInternalDecode(internal bool) DecodingOptionsFunc {
	return func(opts *DecodingOptions) {
		opts.Internal = &internal
	}
}

func WithStrictDecode(strict bool) DecodingOptionsFunc {
	return func(opts *DecodingOptions) {
		opts.Strict = &strict
	}
}

func WithDefaultsDecode(defaults bool) DecodingOptionsFunc {
	return func(opts *DecodingOptions) {
		opts.Default = &defaults
	}
}

func WithListElementsDecoding(listElements bool) DecodingOptionsFunc {
	return func(opts *DecodingOptions) {
		opts.DecodeListElements = &listElements
	}
}

func boolVar(b bool) *bool { // TODO: move to utils or re-use existing fn?
	return &b
}

func defaultDecodeOpts() *DecodingOptions {
	return &DecodingOptions{
		Internal:           boolVar(false),
		Strict:             boolVar(true),
		Default:            boolVar(false),
		DecodeListElements: boolVar(true),
	}
}

func newDecodeOpts(fns ...DecodingOptionsFunc) *DecodingOptions {
	opts := defaultDecodeOpts()
	for _, fn := range fns {
		fn(opts)
	}
	return opts
}

var DecoderClosedError = errors.New("decoder has already been closed")

type streamDecoder struct {
	*schemeAndCodec

	rc         io.ReadCloser // this is the underlying reader of YAMLReader
	yamlReader *yaml.YAMLReader
	decoder    runtime.Decoder //serializer *json.Serializer
	opts       DecodingOptions
	closed     bool // signal whether Close() has been called or not

	// TODO: Add mutexes for thread-safety
}

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
func (d *streamDecoder) Decode() (runtime.Object, error) {
	// Read a YAML document, and return errors (including io.EOF and DecoderClosedError) if found
	doc, err := d.readDoc()
	if err != nil {
		return nil, err
	}
	return d.decode(doc)
}

func (d *streamDecoder) decode(doc []byte) (runtime.Object, error) {
	// Use our own special (e.g. strict, defaulting/non-defaulting) decoder
	obj, _, err := d.decoder.Decode(doc, nil, nil)
	if err != nil {
		// Give the user good errors wrt missing group & version
		return nil, d.handleDecodeError(doc, err)
	}
	if obj == nil {
		return nil, fmt.Errorf("object is nil!")
	}

	// Return the decoded object
	return obj, nil
}

func (d *streamDecoder) handleDecodeError(doc []byte, origErr error) error {
	// Parse the document's TypeMeta information
	gvk, err := yamlmeta.DefaultMetaFactory.Interpret(doc)
	if err != nil {
		return err // TODO: Wrap
	}

	// Check if the group was known. If not, return that specific error
	if !d.scheme.IsGroupRegistered(gvk.Group) {
		return NewUnrecognizedGroupError(
			fmt.Sprintf("for scheme unrecognized API group: %s", gvk.Group),
			*gvk,
			doc,
		)
	}

	// Return a structured error if the group was registered with the scheme but the version was unrecognized
	if !d.scheme.IsVersionRegistered(gvk.GroupVersion()) {
		gvs := d.scheme.PrioritizedVersionsForGroup(gvk.Group)
		return NewUnrecognizedVersionError(
			fmt.Sprintf("for scheme unrecognized API version: %s. Registered GroupVersions: %v", gvk.GroupVersion().String(), gvs),
			*gvk,
			doc,
		)
	}

	// If nothing else, just return the underlying error
	return origErr
}

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
func (d *streamDecoder) DecodeInto(into runtime.Object) error {
	// Read a YAML document, and return errors (including io.EOF and DecoderClosedError) if found
	doc, err := d.readDoc()
	if err != nil {
		return err
	}

	// Use our own special (e.g. strict, defaulting/non-defaulting) decoder
	// This logic is the same as runtime.DecodeInto, but with better error handling
	out, gvk, err := d.decoder.Decode(doc, nil, into)
	if err != nil {
		// Give the user good errors wrt missing group & version
		return d.handleDecodeError(doc, err)
	}
	if out != into {
		return fmt.Errorf("unable to decode %s into %v", gvk, reflect.TypeOf(into))
	}
	return nil
}

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
func (d *streamDecoder) DecodeAll() ([]runtime.Object, error) {
	objs := []runtime.Object{}
	for {
		obj, err := d.Decode()
		if err == io.EOF {
			// If we encountered io.EOF, we know that all is fine and we can exit the for loop and return
			break
		} else if err != nil {
			return nil, err
		}

		// If we asked to decode list elements, and it is a list, go ahead and loop through
		// Otherwise, just add the object to the slice and continue
		if list, ok := obj.(*metav1.List); *d.opts.DecodeListElements && ok {
			for _, item := range list.Items {
				// Decode each part of the list
				listobj, err := d.decode(item.Raw)
				if err != nil {
					return nil, err
				}
				objs = append(objs, listobj)
			}
		} else {
			// A normal, non-list object
			objs = append(objs, obj)
		}

	}
	return objs, nil
}

// readDoc tries to read the next document from the framer
// If the decoder is already closed, it returns DecodedClosedError
// If it encounters an io.EOF, it runs d.Close() and returns io.EOF
func (d *streamDecoder) readDoc() ([]byte, error) {
	// If the decoder is already closed, return DecodedClosedError
	if d.closed {
		return nil, DecoderClosedError
	}

	for {
		// TODO: Maybe use a generic Framer for both reading & writing?
		doc, err := d.yamlReader.Read()
		if err == io.EOF {
			d.Close()
			return nil, io.EOF
		} else if err != nil {
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

func (d *streamDecoder) Close() error {
	d.closed = true
	return d.rc.Close()
}

/*// WithOptions returns a new Decoder with new options, preserving the same data & scheme
// The options are not defaulted, but used as-is. This call MUST happen before any Decode call
func (d *streamDecoder) WithOptions(opts DecodingOptions) Decoder {
	// TODO: Return err-decoder if we've already called any Decode call?
	return newStreamDecoder(d.rc, d.schemeAndCodec, opts)
}*/

func newDecoder(schemeAndCodec *schemeAndCodec, rc io.ReadCloser, opts DecodingOptions) Decoder {
	// The YAML reader supports reading multiple YAML documents
	yamlReader := yaml.NewYAMLReader(bufio.NewReader(rc))

	// Allow both YAML and JSON inputs (JSON is a subset of YAML), and deserialize in strict mode
	s := json.NewSerializerWithOptions(json.DefaultMetaFactory, schemeAndCodec.scheme, schemeAndCodec.scheme, json.SerializerOptions{
		Yaml:   true,
		Strict: *opts.Strict,
	})
	// Construct a codec that uses the strict serializer, but also performs defaulting & conversion
	//decoder := recognizer.NewDecoder(s) // schemeAndCodec.codecs.CodecForVersions(nil, s, nil, runtime.InternalGroupVersioner)
	// decoder := schemeAndCodec.codecs.UniversalDeserializer()

	// Default to preferring the scheme's preferred external version for Decode calls (not DecodeInto)
	var groupVersioner runtime.GroupVersioner = &schemeGroupVersioner{schemeAndCodec.scheme}
	if *opts.Internal {
		// If we asked to always always decode and convert into internal, do it
		groupVersioner = runtime.InternalGroupVersioner
	}

	decoder := newConversionCodecForScheme(schemeAndCodec.scheme, nil, s, nil, groupVersioner, *opts.Default)

	return &streamDecoder{schemeAndCodec, rc, yamlReader, decoder, opts, false}
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

type schemeGroupVersioner struct {
	scheme *runtime.Scheme
}

// KindForGroupVersionKinds returns an internal Kind if one is found, or converts the first provided kind to the internal version.
func (sgv *schemeGroupVersioner) KindForGroupVersionKinds(kinds []schema.GroupVersionKind) (schema.GroupVersionKind, bool) {
	for _, gvk := range kinds {
		for _, gv := range sgv.scheme.PrioritizedVersionsForGroup(gvk.Group) {
			return gv.WithKind(gvk.Kind), true
		}
	}

	return schema.GroupVersionKind{}, false
}

// Identifier implements GroupVersioner interface.
func (schemeGroupVersioner) Identifier() string {
	return "scheme"
}
