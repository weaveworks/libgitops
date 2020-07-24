package serializer

import (
	"bytes"
	"encoding/base64"
	"errors"
	"fmt"

	"github.com/sirupsen/logrus"
	"github.com/weaveworks/libgitops/pkg/serializer/comments"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/kustomize/kyaml/yaml"
)

const preserveCommentsAnnotation = "serializer.libgitops.weave.works/original-data"

// tryToPreserveComments tries to save the original file data (base64-encoded) into an annotation.
// This original file data can be used at encoding-time to preserve comments
func (d *decoder) tryToPreserveComments(doc []byte, obj runtime.Object, ct ContentType) {
	// If the user opted into preserving comments and the format is YAML, proceed
	// If they didn't, return directly
	if !(*d.opts.PreserveComments && ct == ContentTypeYAML) {
		return
	}

	// Convert the object to a metav1.Object (this requires embedding ObjectMeta)
	metaobj, ok := toMetaObject(obj)
	if !ok {
		// If the object doesn't have ObjectMeta embedded, just do nothing
		logrus.Debugf("Couldn't convert object with GVK %q to metav1.Object, although opts.PreserveComments is enabled", obj.GetObjectKind().GroupVersionKind())
		return
	}

	// Preserve the original file content in the annotation
	setAnnotation(metaobj, preserveCommentsAnnotation, base64.StdEncoding.EncodeToString(doc))
}

// tryToPreserveComments tries to locate the possibly-saved original file data in the object's annotation
func (e *encoder) encodeWithCommentSupport(versionEncoder runtime.Encoder, fw FrameWriter, obj runtime.Object, metaobj metav1.Object) error {
	// If the user did not opt into preserving comments, just sanitize ObjectMeta temporarily and and return
	if !*e.opts.PreserveComments {
		// Normal encoding without the annotation (so it doesn't leak by accident)
		return noAnnotationWrapper(metaobj, e.normalEncodeFunc(versionEncoder, fw, obj))
	}

	// The user requested to preserve comments, but content type is not YAML, so log, sanitize and return
	if fw.ContentType() != ContentTypeYAML {
		logrus.Debugf("Asked to preserve comments, but ContentType is not YAML, so ignoring")

		// Normal encoding without the annotation (so it doesn't leak by accident)
		return noAnnotationWrapper(metaobj, e.normalEncodeFunc(versionEncoder, fw, obj))
	}

	// Get the encoded previous file data from the annotation or fall back to "normal" encoding
	encodedPriorData, ok := getAnnotation(metaobj, preserveCommentsAnnotation)
	if !ok {
		// no need to delete the annotation as we know it doesn't exist, just do a normal encode
		return e.normalEncodeFunc(versionEncoder, fw, obj)()
	}

	// Decode the base64-encoded bytes of the original object (including the comments)
	priorData, err := base64.StdEncoding.DecodeString(encodedPriorData)
	if err != nil {
		// fatal error
		return err
	}

	// Unmarshal the original YAML document into a *yaml.RNode, including comments
	priorNode, err := yaml.Parse(string(priorData))
	if err != nil {
		// fatal error
		return err
	}

	// Encode the new object into a temporary buffer, it should not be written as the "final result" to the fw
	buf := new(bytes.Buffer)
	if err := noAnnotationWrapper(metaobj, e.normalEncodeFunc(versionEncoder, NewYAMLFrameWriter(buf), obj)); err != nil {
		// fatal error
		return err
	}
	updatedData := buf.Bytes()

	// Parse the new, upgraded, encoded YAML into *yaml.RNode for addition
	// of comments from prevNode
	afterNode, err := yaml.Parse(string(updatedData))
	if err != nil {
		// fatal error
		return err
	}

	// Copy over comments from the old to the new schema
	if err := comments.CopyComments(priorNode, afterNode, true); err != nil {
		// fatal error
		return err
	}

	// Print the new schema with the old comments kept to the FrameWriter
	_, err = fmt.Fprint(fw, afterNode.MustString())
	// we're done, exit the encode function
	return err
}

func (e *encoder) normalEncodeFunc(versionEncoder runtime.Encoder, fw FrameWriter, obj runtime.Object) func() error {
	return func() error {
		return versionEncoder.Encode(obj, fw)
	}
}

// noAnnotationWrapper temporarily removes the preserveComments annotation before and after running the function
// one example of this function is e.normalEncodeFunc
func noAnnotationWrapper(metaobj metav1.Object, fn func() error) error {
	// If the annotation exists, delete it and defer add it back.
	if val, ok := getAnnotation(metaobj, preserveCommentsAnnotation); ok {
		defer setAnnotation(metaobj, preserveCommentsAnnotation, val)
		deleteAnnotation(metaobj, preserveCommentsAnnotation)
	}
	// If the annotation isn't present, just run the function
	return fn()
}

// GetCommentSource retrieves the YAML tree used as the source for transferring comments for the given runtime.Object.
// This may be used externally to implement e.g. re-parenting of the comment source tree when moving structs around.
func GetCommentSource(obj runtime.Object) (*yaml.RNode, error) {
	// Cast the object to a metav1.Object to get access to annotations.
	// If this fails, the given object does not support storing comments.
	metaObj, ok := toMetaObject(obj)
	if !ok {
		return nil, errors.New("the given object cannot store comments, it is not metav1.ObjectMeta compliant")
	}

	// Fetch the source string for the comments. If this fails, the given object does not have any stored comments.
	sourceStr, ok := getAnnotation(metaObj, preserveCommentsAnnotation)
	if !ok {
		return nil, errors.New("the given object does not have stored comments")
	}

	// Decode the base64-encoded comment source string.
	sourceBytes, err := base64.StdEncoding.DecodeString(sourceStr)
	if err != nil {
		return nil, err
	}

	// Parse the decoded source data into a *yaml.RNode and return it.
	return yaml.Parse(string(sourceBytes))
}

// SetCommentSource sets the given YAML tree as the source for transferring comments for the given runtime.Object.
// This may be used externally to implement e.g. re-parenting of the comment source tree when moving structs around.
func SetCommentSource(obj runtime.Object, source *yaml.RNode) error {
	// Convert the given tree into a string. This also handles the source == nil case.
	str, err := source.String()
	if err != nil {
		return err
	}

	// Cast the object to a metav1.Object to get access to annotations.
	// If this fails, the given object does not support storing comments.
	metaObj, ok := toMetaObject(obj)
	if !ok {
		return errors.New("the given object cannot store comments, it is not metav1.ObjectMeta compliant")
	}

	// base64-encode the comments string.
	encodedStr := base64.StdEncoding.EncodeToString([]byte(str))

	// Set the value of the comments annotation to the encoded string.
	setAnnotation(metaObj, preserveCommentsAnnotation, encodedStr)
	return nil
}
