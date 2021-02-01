package core

import (
	"context"

	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// Note: package core must not depend on any other parts of the libgitops repo, possibly the serializer package as an exception.
// Anything under k8s.io/apimachinery goes though, and important external imports
// like github.com/spf13/afero is also ok. The pretty large sigs.k8s.io/controller-runtime
// import is a bit sub-optimal, though.

// GroupVersionKind aliases
type GroupKind = schema.GroupKind
type GroupVersion = schema.GroupVersion
type GroupVersionKind = schema.GroupVersionKind

// Client-related Object aliases
type Object = client.Object
type ObjectKey = types.NamespacedName
type ObjectList = client.ObjectList
type Patch = client.Patch

// Client-related Option aliases
type ListOption = client.ListOption
type CreateOption = client.CreateOption
type UpdateOption = client.UpdateOption
type PatchOption = client.PatchOption
type DeleteOption = client.DeleteOption
type DeleteAllOfOption = client.DeleteAllOfOption

// Helper functions from client.
var ObjectKeyFromObject = client.ObjectKeyFromObject

// TODO: Investigate if the ObjectRecognizer should return unversioned
// or versioned ObjectID's
type ObjectRecognizer interface {
	ResolveObjectID(ctx context.Context, fileName string, content []byte) (ObjectID, error)
}

// UnversionedObjectID represents an ID for an Object whose version is not known.
// However, the Group, Kind, Name and optionally, Namespace is known and should
// uniquely identify the Object at a specific moment in time.
type UnversionedObjectID interface {
	GroupKind() GroupKind
	ObjectKey() ObjectKey

	WithVersion(version string) ObjectID
}

// ObjectID is a superset of UnversionedObjectID, that also specifies an exact version.
type ObjectID interface {
	UnversionedObjectID

	GroupVersionKind() GroupVersionKind
}

// VersionRef is an interface that describes a reference to a specific version
// of Objects in a Storage or Client.
type VersionRef interface {
	// String returns the commit or branch name.
	String() string
	// IsWritable determines if the VersionRef points to such a state where it
	// is possible to write on top of it, i.e. as in the case of a Git branch.
	//
	// A specific Git commit, however, isn't considered writable, as it points
	// to a specific point in time that can't just be rewritten, (assuming this
	// library only is additive, which it is).
	IsWritable() bool
	// IsZeroValue determines if this VersionRef is the "zero value", which means
	// that the caller should figure out how to handle that the user did not
	// give specific opinions of what version of the Object to get.
	IsZeroValue() bool
}
