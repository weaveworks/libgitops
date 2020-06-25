package serializer

import (
	"bytes"
	"encoding/base64"
	"fmt"

	"github.com/weaveworks/libgitops/pkg/util"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/kustomize/kyaml/comments"
	"sigs.k8s.io/kustomize/kyaml/yaml"
)

type EncodingOptions struct {
	// Use pretty printing when writing to the output. (Default: true)
	// TODO: Fix that sometimes omitempty fields aren't respected
	Pretty *bool
	// Whether to preserve YAML comments internally. Only applicable to ContentTypeYAML framers.
	// Using any other framer will be silently ignored. Usage of this option also requires setting
	// the PreserveComments in EncodingOptions, too. (Default: false)
	PreserveComments *bool

	// TODO: Maybe consider an option to always convert to the preferred version (not just internal)
}

type EncodingOptionsFunc func(*EncodingOptions)

func WithPrettyEncode(pretty bool) EncodingOptionsFunc {
	return func(opts *EncodingOptions) {
		opts.Pretty = &pretty
	}
}

func WithCommentsEncode(comments bool) EncodingOptionsFunc {
	return func(opts *EncodingOptions) {
		opts.PreserveComments = &comments
	}
}

func WithEncodingOptions(newOpts EncodingOptions) EncodingOptionsFunc {
	return func(opts *EncodingOptions) {
		*opts = newOpts
	}
}

func defaultEncodeOpts() *EncodingOptions {
	return &EncodingOptions{
		Pretty:           util.BoolPtr(true),
		PreserveComments: util.BoolPtr(false),
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

	opts EncodingOptions
}

func newEncoder(schemeAndCodec *schemeAndCodec, opts EncodingOptions) Encoder {
	return &encoder{
		schemeAndCodec,
		opts,
	}
}

// Encode encodes the given objects and writes them to the specified FrameWriter.
// The FrameWriter specifies the ContentType. This encoder will automatically convert any
// internal object given to the preferred external groupversion. No conversion will happen
// if the given object is of an external version.
func (e *encoder) Encode(fw FrameWriter, objs ...runtime.Object) error {
	for _, obj := range objs {
		// Get the kind for the given object
		gvk, err := gvkForObject(e.scheme, obj)
		if err != nil {
			return err
		}

		// If the object is internal, convert it to the preferred external one
		if gvk.Version == runtime.APIVersionInternal {
			gvk, err = externalGVKForObject(e.scheme, obj)
			if err != nil {
				return err
			}
		}

		// Encode it
		if err := e.EncodeForGroupVersion(fw, obj, gvk.GroupVersion()); err != nil {
			return err
		}
	}
	return nil
}

// EncodeForGroupVersion encodes the given object for the specific groupversion. If the object
// is not of that version currently it will try to convert. The output bytes are written to the
// FrameWriter. The FrameWriter specifies the ContentType.
func (e *encoder) EncodeForGroupVersion(fw FrameWriter, obj runtime.Object, gv schema.GroupVersion) error {
	// Get the serializer for the media type
	serializerInfo, ok := runtime.SerializerInfoForMediaType(e.codecs.SupportedMediaTypes(), string(fw.ContentType()))
	if !ok {
		return ErrUnsupportedContentType
	}

	// Choose the pretty or non-pretty one
	encoder := serializerInfo.Serializer
	if *e.opts.Pretty {
		encoder = serializerInfo.PrettySerializer
	}

	// Get a version-specific encoder for the specified groupversion
	versionEncoder := e.codecs.EncoderForVersion(encoder, gv)

	// If the user asked to preserve comments and the contenttype is YAML, try to preserve comments
	// If preserving comments was asked for, but the content type isn't YAML, continue with normal operation
	if *e.opts.PreserveComments && fw.ContentType() == ContentTypeYAML {
		err, ok := e.tryToPreserveComments(versionEncoder, fw, obj)
		if ok {
			return err
		}
	}

	// Make sure we sanitize the annotation before encoding
	sanitizeAnnotations(obj)

	// Specialize the encoder for a specific gv and encode the object
	return versionEncoder.Encode(obj, fw)
}

func (e *encoder) tryToPreserveComments(versionEncoder runtime.Encoder, fw FrameWriter, obj runtime.Object) (error, bool) {
	// TODO: Maybe use named returns here?

	// Cast the object to a metav1.Object to get access to annotations
	metaobj, ok := obj.(metav1.Object)
	if !ok { // for objects without ObjectMeta, this will fail. allow that failure.
		// fallback to "normal" encoding
		return nil, false // fmt.Errorf("couldn't convert to metav1.Object") // TODO: maybe typed errors?
	}

	// Get annotations and the specific annotation value we're interested in
	// TODO: This will error if metav1.ObjectMeta is embedded int the object
	// as a pointer (i.e. *metav1.ObjectMeta) and nil
	a := metaobj.GetAnnotations()
	if a == nil {
		// fallback to "normal" encoding
		return nil, false // maybe error?
	}
	encodedPriorData, ok := a[preserveCommentsAnnotation]
	if !ok {
		// fallback to "normal" encoding
		return nil, false // maybe error?
	}

	// Decode the base64-encoded bytes of the original object (including the comments)
	priorData, err := base64.StdEncoding.DecodeString(encodedPriorData)
	if err != nil {
		// fatal error
		return err, true
	}

	// Unmarshal the original YAML document into a *yaml.RNode, including comments
	priorNode, err := yaml.Parse(string(priorData))
	if err != nil {
		// fatal error
		return err, true
	}

	// Make sure we sanitize the annotation before encoding
	sanitizeAnnotations(obj)

	// Encode the new object into a temporary buffer, it should not be written as the "final result" to the fw
	buf := new(bytes.Buffer)
	if err := versionEncoder.Encode(obj, NewYAMLFrameWriter(buf)); err != nil {
		// fatal error
		return err, true
	}
	updatedData := buf.Bytes()

	// Parse the new, upgraded, encoded YAML into *yaml.RNode for addition
	// of comments from prevNode
	afterNode, err := yaml.Parse(string(updatedData))
	if err != nil {
		// fatal error
		return err, true
	}

	// Copy over comments from the old to the new schema
	// TODO: Also preserve comments that are "lost on the way", i.e. on schema changes
	if err := comments.CopyComments(priorNode, afterNode); err != nil {
		// fatal error
		return err, true
	}

	// Print the new schema with the old comments kept to the FrameWriter
	_, err = fmt.Fprint(fw, afterNode.MustString())
	// we're done, exit the encode function
	return err, true
}

func sanitizeAnnotations(obj runtime.Object) {
	// TODO: Also automatically sanitize metadata.creationTimestamp which is otherwise always output?

	// Cast the object to a metav1.Object to get access to annotations
	metaobj, ok := obj.(metav1.Object)
	if !ok { // if the object doesn't have this ObjectMeta, never mind
		return
	}

	// Get annotations and the specific annotation value we're interested in
	// TODO: This will error if metav1.ObjectMeta is embedded int the object
	// as a pointer (i.e. *metav1.ObjectMeta) and nil
	a := metaobj.GetAnnotations()
	if a == nil { // if the object doesn't have any annotations, never mind
		return
	}
	_, ok = a[preserveCommentsAnnotation]
	if !ok { // if the object doesn't have the annotation, never mind
		return
	}

	// Delete the internal annotation
	delete(a, preserveCommentsAnnotation)
	metaobj.SetAnnotations(a)
}
