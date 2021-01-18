package kube

import (
	"errors"
	"fmt"
	"sync"

	"github.com/weaveworks/libgitops/pkg/storage/core"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/sets"
)

// TODO: Make an example component that iterates through all of a raw.Storage's
// or FileFinder's objects, and just reads them, converts them into the current
// hub version.

var (
	// ErrNoSuchNamespace means that the set of namespaces was searched in the
	// system, but the requested namespace wasn't in that list.
	ErrNoSuchNamespace = errors.New("no such namespace in the system")
)

// NamespaceEnforcer implements core.NamespaceEnforcer similarly to how the
// Kubernetes API server behaves.
type NamespaceEnforcer struct{}

var _ core.NamespaceEnforcer = NamespaceEnforcer{}

func (NamespaceEnforcer) RequireSetNamespaceExists() bool { return true }

func (NamespaceEnforcer) EnforceNamespace(obj core.Object, namespaced bool, namespaces sets.String) error {
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

// SimpleRESTMapper is a subset of the meta.RESTMapper interface
type SimpleRESTMapper interface {
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
func RESTMapperToNamespacer(mapper SimpleRESTMapper) core.Namespacer {
	return &restNamespacer{
		mapper:        mapper,
		mappingByType: make(map[schema.GroupKind]*meta.RESTMapping),
		mu:            &sync.RWMutex{},
	}
}

var _ core.Namespacer = &restNamespacer{}

type restNamespacer struct {
	mapper SimpleRESTMapper

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
