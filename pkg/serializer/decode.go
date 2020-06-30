package serializer

import (
	"fmt"
	"io"
	"reflect"

	"github.com/weaveworks/libgitops/pkg/util"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/serializer/json"
	"k8s.io/apimachinery/pkg/runtime/serializer/versioning"
	yamlmeta "k8s.io/apimachinery/pkg/runtime/serializer/yaml"
)

// This is the groupversionkind for the v1.List object
var listGVK = metav1.Unversioned.WithKind("List")

type DecodingOptions struct {
	// Not applicable for Decoder.DecodeInto(). If true, the decoded external object
	// will be converted into its hub (or internal, where applicable) representation. Otherwise, the decoded
	// object will be left in its external representation. (Default: false)
	ConvertToHub *bool
	// Parse the YAML/JSON in strict mode, returning a specific error if the input
	// contains duplicate or unknown fields or formatting errors. (Default: true)
	Strict *bool
	// Automatically default the decoded object. (Default: false)
	Default *bool
	// Only applicable for Decoder.DecodeAll(). If the underlying data contains a v1.List,
	// the items of the list will be traversed, decoded into their respective types, and
	// appended to the returned slice. The v1.List will in this case not be returned.
	// This conversion does NOT support preserving comments. If the given scheme doesn't
	// recognize the v1.List, before using it will be registered automatically. (Default: true)
	DecodeListElements *bool
	// Whether to preserve YAML comments internally. This only works for objects embedding metav1.ObjectMeta.
	// Only applicable to ContentTypeYAML framers.
	// Using any other framer will be silently ignored. Usage of this option also requires setting
	// the PreserveComments in EncodingOptions, too. (Default: false)
	PreserveComments *bool

	// TODO: Add a DecodeUnknown option
}

type DecodingOptionsFunc func(*DecodingOptions)

func WithConvertToHubDecode(convert bool) DecodingOptionsFunc {
	return func(opts *DecodingOptions) {
		opts.ConvertToHub = &convert
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

func WithCommentsDecode(comments bool) DecodingOptionsFunc {
	return func(opts *DecodingOptions) {
		opts.PreserveComments = &comments
	}
}

func WithDecodingOptions(newOpts DecodingOptions) DecodingOptionsFunc {
	return func(opts *DecodingOptions) {
		// TODO: Null-check all of these before using them
		*opts = newOpts
	}
}

func defaultDecodeOpts() *DecodingOptions {
	return &DecodingOptions{
		ConvertToHub:       util.BoolPtr(false),
		Strict:             util.BoolPtr(true),
		Default:            util.BoolPtr(false),
		DecodeListElements: util.BoolPtr(true),
		PreserveComments:   util.BoolPtr(false),
	}
}

func newDecodeOpts(fns ...DecodingOptionsFunc) *DecodingOptions {
	opts := defaultDecodeOpts()
	for _, fn := range fns {
		fn(opts)
	}
	return opts
}

type decoder struct {
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
// If opts.ConvertToHub is true, the decoded external object will be converted into its hub
// 	(or internal, if applicable) representation.
// 	Otherwise, the decoded object will be left in the external representation.
// opts.DecodeListElements is not applicable in this call.
func (d *decoder) Decode(fr FrameReader) (runtime.Object, error) {
	// Read a frame from the FrameReader
	// TODO: Make sure to test the case when doc might contain something, and err is io.EOF
	doc, err := fr.ReadFrame()
	if err != nil {
		return nil, err
	}
	return d.decode(doc, nil, fr.ContentType())
}

func (d *decoder) decode(doc []byte, into runtime.Object, ct ContentType) (runtime.Object, error) {
	// If the scheme doesn't recognize a v1.List, and we enabled opts.DecodeListElements,
	// make the scheme able to decode the v1.List automatically
	if *d.opts.DecodeListElements && !d.scheme.Recognizes(listGVK) {
		d.scheme.AddKnownTypes(metav1.Unversioned, &metav1.List{})
	}

	// Record if this decode call should have runtime.DecodeInto-functionality
	intoGiven := into != nil

	// Use our own special (e.g. strict, defaulting/non-defaulting) decoder
	// TODO: Make sure any possible strict errors are returned/handled properly
	obj, gvk, err := d.decoder.Decode(doc, nil, into)
	if err != nil {
		// Give the user good errors wrt missing group & version
		return nil, d.handleDecodeError(doc, err)
	}

	// Fail fast if object is nil
	if obj == nil {
		return nil, fmt.Errorf("decoded object is nil! Detected gvk is %v", gvk)
	}

	// This logic is the same as in runtime.DecodeInto, and makes sure that if we requested an
	// "into" object, it actually worked
	if intoGiven && obj != into {
		return nil, fmt.Errorf("unable to decode %s into %v", gvk, reflect.TypeOf(into))
	}

	// Try to preserve comments
	d.tryToPreserveComments(doc, obj, ct)

	// Return the decoded object
	return obj, nil
}

// DecodeInto decodes the next document in the FrameReader stream into obj if the types are matching.
// If there are multiple documents in the underlying stream, this call will read one
// 	document and return it. Decode might be invoked for getting new documents until it
// 	returns io.EOF. When io.EOF is reached in a call, the stream is automatically closed.
// The decoded object will automatically be converted into the target one (i.e. one can supply an
// 	ConvertToHub object to this function).
// If the decoded object is for an unrecognized group, or version, UnrecognizedGroupError
// 	or UnrecognizedVersionError might be returned.
// If opts.Default is true, the decoded object will be defaulted.
// If opts.Strict is true, the YAML/JSON will be parsed in strict mode, returning a specific error
// 	if the input contains duplicate or unknown fields or formatting errors. You can check whether
// 	a returned failed because of the strictness using k8s.io/apimachinery/pkg/runtime.IsStrictDecodingError.
// opts.DecodeListElements is not applicable in this call.
// opts.ConvertToHub is not applicable in this call.
func (d *decoder) DecodeInto(fr FrameReader, into runtime.Object) error {
	// Read a frame from the FrameReader.
	// TODO: Make sure to test the case when doc might contain something, and err is io.EOF
	doc, err := fr.ReadFrame()
	if err != nil {
		return err
	}

	// Run the internal decode() and pass the into object
	_, err = d.decode(doc, into, fr.ContentType())
	return err
}

// DecodeAll returns the decoded objects from all documents in the FrameReader stream. The underlying
// stream is automatically closed on io.EOF. io.EOF is never returned from this function.
// If any decoded object is for an unrecognized group, or version, UnrecognizedGroupError
// 	or UnrecognizedVersionError might be returned.
// If opts.Default is true, the decoded objects will be defaulted.
// If opts.Strict is true, the YAML/JSON will be parsed in strict mode, returning a specific error
// 	if the input contains duplicate or unknown fields or formatting errors. You can check whether
// 	a returned failed because of the strictness using k8s.io/apimachinery/pkg/runtime.IsStrictDecodingError.
// If opts.ConvertToHub is true, the decoded external object will be converted into its hub
// 	(or internal, if applicable) representation.
// If opts.DecodeListElements is true and the underlying data contains a v1.List,
// 	the items of the list will be traversed and decoded into their respective types, which are
// 	added into the returning slice. The v1.List will in this case not be returned.
func (d *decoder) DecodeAll(fr FrameReader) ([]runtime.Object, error) {
	objs := []runtime.Object{}
	for {
		obj, err := d.Decode(fr)
		if err == io.EOF {
			// If we encountered io.EOF, we know that all is fine and we can exit the for loop and return
			break
		} else if err != nil {
			return nil, err
		}

		// Extract possibly nested objects within the one we got (e.g. unwrapping lists if asked to),
		// or just no-op and return the object given for addition to the larger list
		nestedObjs, err := d.extractNestedObjects(obj, fr.ContentType())
		if err != nil {
			return nil, err
		}
		objs = append(objs, nestedObjs...)
	}
	return objs, nil
}

func (d *decoder) handleDecodeError(doc []byte, origErr error) error {
	// Parse the document's TypeMeta information
	gvk, err := yamlmeta.DefaultMetaFactory.Interpret(doc)
	if err != nil {
		return fmt.Errorf("failed to interpret TypeMeta from the given the YAML: %v. Decode error was: %w", err, origErr)
	}

	// TODO: Unit test that typed errors are returned properly

	// Check if the group was known. If not, return that specific error
	if !d.scheme.IsGroupRegistered(gvk.Group) {
		return NewUnrecognizedGroupError(*gvk, origErr)
	}

	// Return a structured error if the group was registered with the scheme but the version was unrecognized
	if !d.scheme.IsVersionRegistered(gvk.GroupVersion()) {
		gvs := d.scheme.PrioritizedVersionsForGroup(gvk.Group)
		return NewUnrecognizedVersionError(gvs, *gvk, origErr)
	}

	// Return a structured error if the kind is not known
	if !d.scheme.Recognizes(*gvk) {
		return NewUnrecognizedKindError(*gvk, origErr)
	}

	// If nothing else, just return the underlying error
	return origErr
}

func (d *decoder) extractNestedObjects(obj runtime.Object, ct ContentType) ([]runtime.Object, error) {
	// If we didn't ask for list-unwrapping functionality, return directly
	if !*d.opts.DecodeListElements {
		return []runtime.Object{obj}, nil
	}

	// Try to cast the object to a v1.List. If the object isn't a list, just return it
	list, ok := obj.(*metav1.List)
	if !ok {
		return []runtime.Object{obj}, nil
	}

	// Loop through the list, and decode every item. Return the final list
	var objs []runtime.Object
	for _, item := range list.Items {
		// Decode each item of the list
		listobj, err := d.decode(item.Raw, nil, ct)
		if err != nil {
			return nil, err
		}
		objs = append(objs, listobj)
	}
	return objs, nil
}

func newDecoder(schemeAndCodec *schemeAndCodec, opts DecodingOptions) Decoder {
	// Allow both YAML and JSON inputs (JSON is a subset of YAML), and deserialize in strict mode
	s := json.NewSerializerWithOptions(json.DefaultMetaFactory, schemeAndCodec.scheme, schemeAndCodec.scheme, json.SerializerOptions{
		Yaml:   true,
		Strict: *opts.Strict,
	})

	codec := newConversionCodecForScheme(schemeAndCodec.scheme, nil, s, nil, runtime.InternalGroupVersioner, *opts.Default, *opts.ConvertToHub)

	return &decoder{schemeAndCodec, codec, opts}
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
	performConversion bool,
) runtime.Codec {
	var defaulter runtime.ObjectDefaulter
	if performDefaulting {
		defaulter = scheme
	}
	convertor := newObjectConvertor(scheme, performConversion)
	return versioning.NewCodec(encoder, decoder, convertor, scheme, scheme, defaulter, encodeVersion, decodeVersion, scheme.Name())
}
