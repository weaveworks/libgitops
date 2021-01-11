package storage

import (
	"errors"
	"fmt"
	"sync"

	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/sets"
)

var (
	// ErrNoSuchNamespace means that the set of namespaces was searched in the
	// system, but the requested namespace wasn't in that list.
	ErrNoSuchNamespace = errors.New("no such namespace in the system")
)

// NamespaceEnforcer enforces a namespace policy for the Storage.
type NamespaceEnforcer interface {
	// RequireNamespaceExists specifies whether the namespace must exist in the system.
	// For example, Kubernetes requires this by default.
	RequireNamespaceExists() bool
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

// K8sNamespaceEnforcer implements NamespaceEnforcer similarly to how the API server behaves.
type K8sNamespaceEnforcer struct{}

var _ NamespaceEnforcer = K8sNamespaceEnforcer{}

func (K8sNamespaceEnforcer) RequireNamespaceExists() bool { return true }

func (K8sNamespaceEnforcer) EnforceNamespace(obj Object, namespaced bool, namespaces sets.String) error {
	ns := obj.GetNamespace()
	if !namespaced {
		// If a namespace was set, it should be sanitized.
		if len(ns) != 0 {
			obj.SetNamespace("")
		}
		return nil
	}
	// The resource is namespaced.
	// If it is empty, set it to the default namespace.
	if len(ns) == 0 {
		obj.SetNamespace(metav1.NamespaceDefault)
		return nil
	}
	// If the namespace field is set, but it doesn't exist in the set, error
	if !namespaces.Has(ns) {
		return fmt.Errorf("%w: %q", ErrNoSuchNamespace, ns)
	}
	return nil
}

// Namespacer is an interface that lets the caller know if a GroupKind is namespaced
// or not. There are two ready-made implementations:
// 1. RESTMapperToNamespacer
// 2. NewStaticNamespacer
type Namespacer interface {
	// IsNamespaced returns true if the GroupKind is a namespaced type
	IsNamespaced(gk schema.GroupKind) (bool, error)
}

// RESTMapper is a subset of the meta.RESTMapper interface
type RESTMapper interface {
	// RESTMapping identifies a preferred resource mapping for the provided group kind.
	RESTMapping(gk schema.GroupKind, versions ...string) (*meta.RESTMapping, error)
}

// RESTMapperToNamespacer implements the Namespacer interface by fetching (and caching) data
// from the given RESTMapper interface, that is compatible with any meta.RESTMapper implementation.
// This allows you to e.g. pass in a meta.RESTMapper yielded from
// sigs.k8s.io/controller-runtime/pkg/client/apiutil.NewDiscoveryRESTMapper(c *rest.Config), or
// k8s.io/client-go/restmapper.NewDiscoveryRESTMapper(groups []*restmapper.APIGroupResources)
// in order to look up namespacing information from either a running API server, or statically, from
// the list of restmapper.APIGroupResources.
func RESTMapperToNamespacer(mapper RESTMapper) Namespacer {
	return &restNamespacer{
		mapper:        mapper,
		mappingByType: make(map[schema.GroupKind]*meta.RESTMapping),
		mu:            &sync.RWMutex{},
	}
}

var _ Namespacer = &restNamespacer{}

type restNamespacer struct {
	mapper RESTMapper

	mappingByType map[schema.GroupKind]*meta.RESTMapping
	mu            *sync.RWMutex
}

func (n *restNamespacer) IsNamespaced(gk schema.GroupKind) (bool, error) {
	m, err := n.getMapping(gk)
	if err != nil {
		return false, err
	}
	return mappingNamespaced(m), nil
}

func (n *restNamespacer) getMapping(gk schema.GroupKind) (*meta.RESTMapping, error) {
	n.mu.RLock()
	mapping, ok := n.mappingByType[gk]
	n.mu.RUnlock()
	// If already cached, we're ok
	if ok {
		return mapping, nil
	}

	// Write the mapping info to our cache
	n.mu.Lock()
	defer n.mu.Unlock()
	m, err := n.mapper.RESTMapping(gk)
	if err != nil {
		return nil, err
	}
	n.mappingByType[gk] = m
	return m, nil
}

func mappingNamespaced(mapping *meta.RESTMapping) bool {
	return mapping.Scope.Name() == meta.RESTScopeNameNamespace
}

// NewStaticNamespacer has a default policy, which is that objects are in general namespaced
// (defaultToNamespaced == true), or that they are in general root-scoped (defaultToNamespaced == false).
// To the default policy, exceptions can be added, so that for that GroupKind, the default
// policy is reversed.
func NewStaticNamespacer(defaultToNamespaced bool, exceptions ...schema.GroupKind) Namespacer {
	return &staticNamespacedInfo{defaultToNamespaced, exceptions}
}

var _ Namespacer = &staticNamespacedInfo{}

type staticNamespacedInfo struct {
	defaultToNamespaced bool
	exceptions          []schema.GroupKind
}

func (n *staticNamespacedInfo) IsNamespaced(gk schema.GroupKind) (bool, error) {
	if n.defaultToNamespaced {
		// namespace by default, the gks list is a list of root-scoped entities
		return !n.gkIsException(gk), nil
	}
	// root by default, the gks in the list are namespaced
	return n.gkIsException(gk), nil
}

func (n *staticNamespacedInfo) gkIsException(target schema.GroupKind) bool {
	for _, gk := range n.exceptions {
		if gk == target {
			return true
		}
	}
	return false
}
