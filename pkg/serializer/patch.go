package serializer

import (
	"bytes"
	"encoding/json"
	"errors"

	"github.com/weaveworks/libgitops/pkg/util/patch"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/strategicpatch"
	openapi "k8s.io/kube-openapi/pkg/util/proto"
)

type Patcher interface {
	// ApplyOnStruct applies the given patch (JSON-encoded) using the given BytePatcher
	// (that knows how to operate on that kind of patch type) into obj.
	//
	// obj MUST be a typed object. Unversioned, partial or unstructured objects are not
	// supported. For those use-cases, convert your object into an unstructured one, and
	// pass it to ApplyOnUnstructured.
	//
	// obj MUST NOT be an internal type. If you operate on an internal object as your "hub",
	// convert the object yourself first to the GroupVersion of the patch bytes, and then
	// convert back after this call.
	//
	// In case the patch would require knowledge about the schema (e.g. StrategicMergePatch),
	// this function looks that metadata up using reflection of obj.
	ApplyOnStruct(bytePatcher patch.BytePatcher, patch []byte, obj runtime.Object) error

	// ApplyOnUnstructured applies the given patch (JSON-encoded) using the given BytePatcher
	// (that knows how to operate on that kind of patch type) into the unstructured obj.
	//
	// If knowledge about the schema is required by the patch type (e.g. StrategicMergePatch),
	// it is the liability of the caller to provide an OpenAPI schema.
	ApplyOnUnstructured(bytePatcher patch.BytePatcher, patch []byte, obj runtime.Unstructured, schema openapi.Schema) error
}

type patcher struct {
	*schemeAndCodec
}

// ApplyOnStruct applies the given patch (JSON-encoded) using the given BytePatcher
// (that knows how to operate on that kind of patch type) into obj.
//
// obj MUST be a typed object. Unversioned, partial or unstructured objects are not
// supported. For those use-cases, convert your object into an unstructured one, and
// pass it to ApplyOnUnstructured.
//
// obj MUST NOT be an internal type. If you operate on an internal object as your "hub",
// convert the object yourself first to the GroupVersion of the patch bytes, and then
// convert back after this call.
//
// In case the patch would require knowledge about the schema (e.g. StrategicMergePatch),
// this function looks that metadata up using reflection of obj.
func (p *patcher) ApplyOnStruct(bytePatcher patch.BytePatcher, patch []byte, obj runtime.Object) error {
	// Require that obj is typed
	if !IsTyped(obj, p.scheme) {
		return errors.New("obj must be typed")
	}
	// Get the GVK so we can check if obj is internal
	gvk, err := GVKForObject(p.scheme, obj)
	if err != nil {
		return err
	}
	// It must not be internal, as we will encode it soon.
	if gvk.Version == runtime.APIVersionInternal {
		return errors.New("obj must not be internal")
	}

	// Create a non-pretty encoder
	encopt := *defaultEncodeOpts().ApplyOptions([]EncodeOption{PrettyEncode(false)})
	enc := newEncoder(p.schemeAndCodec, encopt)
	// Encode without conversion to the buffer
	var buf bytes.Buffer
	if err := enc.EncodeForGroupVersion(NewJSONFrameWriter(&buf), obj, gvk.GroupVersion()); err != nil {
		return err
	}

	// Get the schema in case needed by the BytePatcher
	schema, err := strategicpatch.NewPatchMetaFromStruct(obj)
	if err != nil {
		return err
	}

	// Apply the patch, and get the new JSON out
	newJSON, err := bytePatcher.Apply(buf.Bytes(), patch, schema)
	if err != nil {
		return err
	}

	// Decode into the object to apply the changes
	fr := NewSingleFrameReader(newJSON, ContentTypeJSON)
	dec := newDecoder(p.schemeAndCodec, *defaultDecodeOpts())
	if err := dec.DecodeInto(fr, obj); err != nil {
		return err
	}

	return nil
}

func (p *patcher) ApplyOnUnstructured(bytePatcher patch.BytePatcher, patch []byte, obj runtime.Unstructured, schema openapi.Schema) error {
	// Marshal the object to form the source JSON
	sourceJSON, err := json.Marshal(obj)
	if err != nil {
		return err
	}

	// Conditionally get the schema from the provided OpenAPI spec
	var patchMeta strategicpatch.LookupPatchMeta
	if schema != nil {
		patchMeta = strategicpatch.NewPatchMetaFromOpenAPI(schema)
	}

	// Apply the patch, and get the new JSON out
	newJSON, err := bytePatcher.Apply(sourceJSON, patch, patchMeta)
	if err != nil {
		return err
	}

	// Decode back into obj
	return json.Unmarshal(newJSON, obj)
}
