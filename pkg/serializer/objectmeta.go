package serializer

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
)

// toMetaObject converts a runtime.Object to a metav1.Object (containing methods that allow modification of
// e.g. annotations, labels, name, namespaces, etc.), and reports whether this cast was successful
func toMetaObject(obj runtime.Object) (metav1.Object, bool) {
	// Check if the object has ObjectMeta embedded. If it does, it can be casted to
	// an ObjectMetaAccessor, which allows us to get operate directly on the ObjectMeta field
	acc, ok := obj.(metav1.ObjectMetaAccessor)
	if !ok {
		return nil, false
	}

	// If, by accident, someone embedded ObjectMeta in their object as a pointer, and forgot to set it, this
	// check makes sure we don't get a nil dereference panic later
	om, ok := acc.GetObjectMeta().(*metav1.ObjectMeta)
	if !ok || om == nil {
		return nil, false
	}

	// Perform the cast, we can now be sure that ObjectMeta is embedded and non-nil
	metaObj, ok := obj.(metav1.Object)
	return metaObj, ok
}

func getAnnotation(metaObj metav1.Object, key string) (string, bool) {
	// Get the annotations map
	a := metaObj.GetAnnotations()
	if a == nil {
		return "", false
	}

	// Get the value like normal and return
	val, ok := a[key]
	return val, ok
}

func setAnnotation(metaObj metav1.Object, key, val string) {
	// Get the annotations map
	a := metaObj.GetAnnotations()

	// If the annotations are nil, create a new map
	if a == nil {
		a = map[string]string{}
	}

	// Set the key-value mapping and write back to the object
	a[key] = val
	// This wouldn't be needed if a was non-nil (as maps are reference types), but in the case
	// of the map being nil, this is a must in order to apply the change
	metaObj.SetAnnotations(a)
}

func deleteAnnotation(metaObj metav1.Object, key string) {
	// Get the annotations map
	a := metaObj.GetAnnotations()

	// If the object doesn't have any annotations or that specific one, never mind
	if a == nil {
		return
	}
	_, ok := a[key]
	if !ok {
		return
	}

	// Delete the internal annotation and write back to the object.
	// The map is passed by reference so this automatically updates the underlying annotations object.
	delete(a, key)
}
