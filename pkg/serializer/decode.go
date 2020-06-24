package serializer

import (
	"fmt"
	"io"
	"reflect"

	"github.com/weaveworks/libgitops/pkg/util"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/runtime/serializer/json"
	"k8s.io/apimachinery/pkg/runtime/serializer/versioning"
	yamlmeta "k8s.io/apimachinery/pkg/runtime/serializer/yaml"
)

type DecodingOptions struct {
	// Not applicable for Decoder.DecodeInto(). If true, the decoded external object
	// will be converted into its internal representation. Otherwise, the decoded
	// object will be left in its external representation. (Default: false)
	// TODO: This should be called sth like ConvertToHub!
	Internal *bool
	// Parse the YAML/JSON in strict mode, returning a specific error if the input
	// contains duplicate or unknown fields or formatting errors. (Default: true)
	Strict *bool
	// Automatically default the decoded object. (Default: false)
	Default *bool
	// Only applicable for Decoder.DecodeAll(). If the underlying data contains a v1.List,
	// the items of the list will be traversed, decoded into their respective types, and
	// appended to the returned slice. The v1.List will in this case not be returned. (Default: true)
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

func WithDecodingOptions(newOpts DecodingOptions) DecodingOptionsFunc {
	return func(opts *DecodingOptions) {
		*opts = newOpts
	}
}

func defaultDecodeOpts() *DecodingOptions {
	return &DecodingOptions{
		Internal:           util.BoolPtr(false),
		Strict:             util.BoolPtr(true),
		Default:            util.BoolPtr(false),
		DecodeListElements: util.BoolPtr(true),
	}
}

func newDecodeOpts(fns ...DecodingOptionsFunc) *DecodingOptions {
	opts := defaultDecodeOpts()
	for _, fn := range fns {
		fn(opts)
	}
	return opts
}

type streamDecoder struct {
	*schemeAndCodec

	decoder runtime.Decoder
	opts    DecodingOptions
}

// Decode returns the decoded object from the next document in the FrameReader stream.
// If there are multiple documents in the underlying stream, this call will read one
// 	document and return it. Decode might be invoked for getting new documents until it
// 	returns io.EOF. When io.EOF is reached in a call, the stream is automatically closed.
// If the decoded object is for an unrecognized group, or version, UnrecognizedGroupError
// 	or UnrecognizedVersionError might be returned.
// If opts.Default is true, the decoded object will be defaulted.
// If opts.Strict is true, the YAML/JSON will be parsed in strict mode, returning a specific error
// 	if the input contains duplicate or unknown fields or formatting errors. You can check whether
// 	a returned failed because of the strictness using k8s.io/apimachinery/pkg/runtime.IsStrictDecodingError.
// If opts.Internal is true, the decoded external object will be converted into its internal representation.
// 	Otherwise, the decoded object will be left in the external representation.
// opts.DecodeListElements is not applicable in this call.
func (d *streamDecoder) Decode(fr FrameReader) (runtime.Object, error) {
	// Read a frame from the FrameReader
	// TODO: Make sure to test the case when doc might contain something, and err is io.EOF
	doc, err := fr.ReadFrame()
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

// DecodeInto decodes the next document in the FrameReader stream into obj if the types are matching.
// If there are multiple documents in the underlying stream, this call will read one
// 	document and return it. Decode might be invoked for getting new documents until it
// 	returns io.EOF. When io.EOF is reached in a call, the stream is automatically closed.
// The decoded object will automatically be converted into the target one (i.e. one can supply an
// 	internal object to this function).
// If the decoded object is for an unrecognized group, or version, UnrecognizedGroupError
// 	or UnrecognizedVersionError might be returned.
// If opts.Default is true, the decoded object will be defaulted.
// If opts.Strict is true, the YAML/JSON will be parsed in strict mode, returning a specific error
// 	if the input contains duplicate or unknown fields or formatting errors. You can check whether
// 	a returned failed because of the strictness using k8s.io/apimachinery/pkg/runtime.IsStrictDecodingError.
// opts.DecodeListElements is not applicable in this call.
// opts.Internal is not applicable in this call.
func (d *streamDecoder) DecodeInto(fr FrameReader, into runtime.Object) error {
	// Read a frame from the FrameReader.
	// TODO: Make sure to test the case when doc might contain something, and err is io.EOF
	doc, err := fr.ReadFrame()
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

// DecodeAll returns the decoded objects from all documents in the FrameReader stream. The underlying
// stream is automatically closed on io.EOF. io.EOF is never returned from this function.
// If any decoded object is for an unrecognized group, or version, UnrecognizedGroupError
// 	or UnrecognizedVersionError might be returned.
// If opts.Default is true, the decoded objects will be defaulted.
// If opts.Strict is true, the YAML/JSON will be parsed in strict mode, returning a specific error
// 	if the input contains duplicate or unknown fields or formatting errors. You can check whether
// 	a returned failed because of the strictness using k8s.io/apimachinery/pkg/runtime.IsStrictDecodingError.
// If opts.Internal is true, the decoded external object will be converted into their internal representation.
// 	Otherwise, the decoded objects will be left in their external representation.
// If opts.DecodeListElements is true and the underlying data contains a v1.List,
// 	the items of the list will be traversed and decoded into their respective types, which are
// 	added into the returning slice. The v1.List will in this case not be returned.
func (d *streamDecoder) DecodeAll(fr FrameReader) ([]runtime.Object, error) {
	objs := []runtime.Object{}
	for {
		obj, err := d.Decode(fr)
		if err == io.EOF {
			// If we encountered io.EOF, we know that all is fine and we can exit the for loop and return
			break
		} else if err != nil {
			return nil, err
		}

		// If we asked to decode list elements, and it is a list, go ahead and loop through
		// Otherwise, just add the object to the slice and continue
		// TODO: This requires scheme.AddKnownTypes(metav1.Unversioned, &metav1.List{})
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

func newDecoder(schemeAndCodec *schemeAndCodec, opts DecodingOptions) Decoder {
	// Allow both YAML and JSON inputs (JSON is a subset of YAML), and deserialize in strict mode
	s := json.NewSerializerWithOptions(json.DefaultMetaFactory, schemeAndCodec.scheme, schemeAndCodec.scheme, json.SerializerOptions{
		Yaml:   true,
		Strict: *opts.Strict,
	})

	// Default to preferring the scheme's preferred external version for Decode calls (not DecodeInto)
	var groupVersioner runtime.GroupVersioner = &schemeGroupVersioner{schemeAndCodec.scheme}
	if *opts.Internal {
		// If we asked to always always decode and convert into internal, do it
		groupVersioner = runtime.InternalGroupVersioner
	}

	decoder := newConversionCodecForScheme(schemeAndCodec.scheme, nil, s, nil, groupVersioner, *opts.Default)

	return &streamDecoder{schemeAndCodec, decoder, opts}
}

// newConversionCodecForScheme is a convenience method for callers that are using a scheme.
// This is very similar to apimachinery/pkg/serializer/versioning.NewDefaultingCodecForScheme
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
	// TODO: Fix the case when there are two external versions with no direct conversion between each other
	// and one decodes the old version.
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
