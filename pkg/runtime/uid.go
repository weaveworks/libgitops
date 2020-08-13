package runtime

/*
// UID represents an unique ID for a type
type UID string

var _ fmt.Stringer = UID("")

// String returns the UID in string representation
func (u UID) String() string {
	return string(u)
}

// This unmarshaler enables the UID to be passed in as an
// unquoted string in JSON. Upon marshaling, quotes will
// be automatically added.
func (u *UID) UnmarshalJSON(b []byte) error {
	if !utf8.Valid(b) {
		return fmt.Errorf("invalid UID string: %s", b)
	}

	uid, err := strconv.Unquote(string(b))
	if err != nil {
		return err
	}

	*u = UID(uid)
	return nil
}


// PartialObjectList is a list of many pointers to PartialObject items
//type PartialObjectList []PartialObject

// TypeMeta is an alias for the k8s/apimachinery TypeMeta with some additional methods
/*type TypeMeta struct {
	metav1.TypeMeta
}

// This is a helper for partialObject generation
// TODO: Maybe we'll solve the lack of this method in upstream metav1.TypeMeta using
// GetObjectKind(), which actually returns a pointer to TypeMeta behind the ObjectKind interface
func (t *TypeMeta) GetTypeMeta() *TypeMeta {
	return t
}

func (t *TypeMeta) GetKind() Kind {
	return Kind(t.Kind)
}

func (t *TypeMeta) GroupVersionKind() schema.GroupVersionKind {
	return t.TypeMeta.GetObjectKind().GroupVersionKind()
}

func (t *TypeMeta) SetGroupVersionKind(gvk schema.GroupVersionKind) {
	t.TypeMeta.GetObjectKind().SetGroupVersionKind(gvk)
}*/

// TODO: Do we really need this anymore?
/*type Kind string

var _ fmt.Stringer = Kind("")

// Returns a string representation of the Kind suitable for sentences
func (k Kind) String() string {
	b := []byte(k)

	// Ignore TLAs
	if len(b) > 3 {
		b[0] = bytes.ToLower(b[:1])[0]
	}

	return string(b)
}

// Returns a title case string representation of the Kind
func (k Kind) Title() string {
	return string(k)
}

// Returns a lowercase string representation of the Kind
func (k Kind) Lower() string {
	return string(bytes.ToLower([]byte(k)))
}

// Returns a Kind parsed from the given string
func ParseKind(input string) Kind {
	b := bytes.ToUpper([]byte(input))

	// Leave TLAs as uppercase
	if len(b) > 3 {
		b = append(b[:1], bytes.ToLower(b[1:])...)
	}

	return Kind(b)
}*/

// ObjectMeta have to be embedded into any serializable object.
// It provides the .GetName() and .GetUID() methods that help
// implement the Object interface
// TODO: Switch over to metav1.ObjectMeta?
/*type ObjectMeta struct {
	Name        string            `json:"name"`
	UID         UID               `json:"uid,omitempty"`
	Created     Time              `json:"created"`
	Labels      map[string]string `json:"labels,omitempty"`
	Annotations map[string]string `json:"annotations,omitempty"`
}

// This is a helper for partialObject generation
func (o *ObjectMeta) GetObjectMeta() *ObjectMeta {
	return o
}

// GetName returns the name of the Object
func (o *ObjectMeta) GetName() string {
	return o.Name
}

// SetName sets the name of the Object
func (o *ObjectMeta) SetName(name string) {
	o.Name = name
}

// GetUID returns the UID of the Object
func (o *ObjectMeta) GetUID() UID {
	return o.UID
}

// SetUID sets the UID of the Object
func (o *ObjectMeta) SetUID(uid UID) {
	o.UID = uid
}

// GetCreated returns when the Object was created
func (o *ObjectMeta) GetCreated() Time {
	return o.Created
}

// SetCreated sets the creation time of the Object
func (o *ObjectMeta) SetCreated(t Time) {
	o.Created = t
}

// GetLabel returns the label value for the key
func (o *ObjectMeta) GetLabel(key string) string {
	if o.Labels == nil {
		return ""
	}
	return o.Labels[key]
}

// SetLabel sets a label value for a key
func (o *ObjectMeta) SetLabel(key, value string) {
	if o.Labels == nil {
		o.Labels = map[string]string{}
	}
	o.Labels[key] = value
}

// GetAnnotation returns the label value for the key
func (o *ObjectMeta) GetAnnotation(key string) string {
	if o.Annotations == nil {
		return ""
	}
	return o.Annotations[key]
}

// SetAnnotation sets a label value for a key
func (o *ObjectMeta) SetAnnotation(key, value string) {
	if o.Annotations == nil {
		o.Annotations = map[string]string{}
	}
	o.Annotations[key] = value
}*/

/*GetTypeMeta() *TypeMeta
GetObjectMeta() *ObjectMeta

//GetKind() Kind
//GroupVersionKind() schema.GroupVersionKind
//SetGroupVersionKind(schema.GroupVersionKind)

GetName() string
SetName(string)

GetUID() UID
SetUID(UID)

GetCreated() Time
SetCreated(t Time)

GetLabel(key string) string
SetLabel(key, value string)

GetAnnotation(key string) string
SetAnnotation(key, value string)*/
