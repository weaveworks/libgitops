package core

import (
	"context"
	"errors"
	"fmt"

	"github.com/weaveworks/libgitops/pkg/serializer"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// KubeObjectRecognizer implements ObjectRecognizer.
var _ ObjectRecognizer = &KubeObjectRecognizer{}

// KubeObjectRecognizer is a simple implementation of ObjectRecognizer, that
// decodes the given byte content with the assumption that it is YAML (which covers
// both YAML and JSON formats) into a *metav1.PartialObjectMetadata, which allows
// extracting the ObjectID from any Kubernetes API Machinery-compatible Object.
//
// This operation works even though *metav1.PartialObjectMetadata is not registered
// with the underlying Scheme in any way.
type KubeObjectRecognizer struct {
	// Decoder is a required field in order for ResolveObjectID to function.
	Decoder serializer.Decoder
	// AllowUnrecognized controls whether this implementation allows recognizing
	// GVK combinations not known to the underlying Scheme. Default: false
	AllowUnrecognized bool
}

func (r *KubeObjectRecognizer) ResolveObjectID(_ context.Context, _ string, content []byte) (ObjectID, error) {
	if r.Decoder == nil {
		return nil, errors.New("programmer error: KubeObjectRecognizer.Decoder is nil")
	}
	metaObj := &metav1.PartialObjectMetadata{}
	err := r.Decoder.DecodeInto(
		serializer.NewSingleFrameReader(content, serializer.ContentTypeYAML),
		metaObj,
	)
	if err != nil {
		return nil, err
	}
	// Validate the object info
	gvk := metaObj.GroupVersionKind()
	if gvk.Group == "" && gvk.Version == "" {
		return nil, fmt.Errorf(".apiVersion field must not be empty")
	}
	if gvk.Kind == "" {
		return nil, fmt.Errorf(".kind field must not be empty")
	}
	if metaObj.Kind == "" {
		return nil, fmt.Errorf(".metadata.name field must not be empty")
	}
	if !r.AllowUnrecognized && !r.Decoder.SchemeLock().Scheme().Recognizes(gvk) {
		return nil, fmt.Errorf("GroupVersionKind %v not recognized by the scheme", gvk)
	}

	return NewObjectID(metaObj.GroupVersionKind(), ObjectKeyFromObject(metaObj)), nil
}
