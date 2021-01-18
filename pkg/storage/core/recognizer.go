package core

import (
	"context"
	"errors"

	"github.com/weaveworks/libgitops/pkg/serializer"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// SerializerObjectRecognizer implements ObjectRecognizer.
var _ ObjectRecognizer = &SerializerObjectRecognizer{}

// SerializerObjectRecognizer is a simple implementation of ObjectRecognizer, that
// decodes the given byte content with the assumption that it is YAML (which covers
// both YAML and JSON formats) into a *metav1.PartialObjectMetadata, which allows
// extracting the ObjectID from any Kubernetes API Machinery-compatible Object.
//
// This operation works even though *metav1.PartialObjectMetadata is not registered
// with the underlying Scheme in any way.
type SerializerObjectRecognizer struct {
	// Serializer is a required field in order for ResolveObjectID to function.
	Serializer serializer.Serializer
}

func (r *SerializerObjectRecognizer) ResolveObjectID(_ context.Context, _ string, content []byte) (ObjectID, error) {
	if r.Serializer == nil {
		return nil, errors.New("programmer error: SerializerObjectRecognizer.Serializer is nil")
	}
	metaObj := &metav1.PartialObjectMetadata{}
	err := r.Serializer.Decoder().DecodeInto(
		serializer.NewSingleFrameReader(content, serializer.ContentTypeYAML),
		metaObj,
	)
	if err != nil {
		return nil, err
	}
	return NewObjectID(metaObj.GroupVersionKind(), ObjectKeyFromObject(metaObj)), nil
}
