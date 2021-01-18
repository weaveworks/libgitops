package core

import (
	"context"

	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/sets"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// Note: package core must not depend on any other parts of the libgitops repo, possibly the serializer package as an exception.
// Anything under k8s.io/apimachinery goes though, and important external imports
// like github.com/spf13/afero is also ok. The pretty large sigs.k8s.io/controller-runtime
// import is a bit sub-optimal, though.

// GroupVersionKind aliases
type GroupKind = schema.GroupKind
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

// NamespaceEnforcer enforces a namespace policy for the Storage.
type NamespaceEnforcer interface {
	// RequireSetNamespaceExists specifies whether the namespace must exist in the system.
	// For example, Kubernetes requires this by default.
	RequireSetNamespaceExists() bool
	// EnforceNamespace operates on the object to make it conform with a given set of rules.
	// If RequireNamespaceExists() is true, all the namespaces available in the system must
	// be passed to namespaces.
	// For example, Kubernetes enforces the following rules:
	// Namespaced resources:
	// 		If .metadata.namespace == "": .metadata.namespace = "default"
	// 		If .metadata.namespace != "": Make sure there is such a namespace, and use it in that case
	// Non-namespaced resources:
	//		If .metadata.namespace != "": .metadata.namespace = ""
	EnforceNamespace(obj Object, namespaced bool, namespaces sets.String) error
}

// Namespacer is an interface that lets the caller know if a GroupKind is namespaced
// or not. There are two ready-made implementations:
// 1. RESTMapperToNamespacer
// 2. NewStaticNamespacer
type Namespacer interface {
	// IsNamespaced returns true if the GroupKind is a namespaced type
	IsNamespaced(gk schema.GroupKind) (bool, error)
}

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

type VersionRef interface {
	IsZero() bool
	String() string
	Type() VersionRefType
}

// VersionRefType specifies if the VersionRef is a commit (i.e. a read-only snapshot), or
// a writable branch. The terminology here is similar to that of Git, so people feel familiar
// with the concepts, but there is not requirement to use Git.
type VersionRefType int

const (
	VersionRefTypeCommit VersionRefType = 1 + iota
	VersionRefTypeBranch
)
