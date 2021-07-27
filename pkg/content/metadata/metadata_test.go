package metadata

import (
	"bufio"
	"bytes"
	"fmt"
	"mime"
	"net/textproto"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"k8s.io/apimachinery/pkg/runtime/serializer/json"
	"sigs.k8s.io/kustomize/kyaml/kio"
	"sigs.k8s.io/yaml"
)

func TestMIME(t *testing.T) {
	for _, part := range strings.Split("text/html, application/xhtml+xml, application/xml;q=0.9, image/webp, */*;q=0.8", ",") {
		t.Error(mime.ParseMediaType(part))
	}
}

func TestTypePrint(t *testing.T) {
	t.Error(fmt.Printf("%T\n", bytes.NewBuffer(nil)))
	t.Error(fmt.Printf("%T\n", json.Framer.NewFrameReader(nil)))
}

func TestK8sYAML(t *testing.T) {
	c := []byte("\n---\n\n---\n    f   :      fo\n\n---\n   \n---\nbar: true") //[]byte("\n---\nfoo:\n- bar: true")

	/*var obj interface{}
	b, err := yaml.YAMLToJSON(c)
	t.Error(string(b), err)
	err = yaml.Unmarshal(c, &obj)*/
	/*for _, subobj := range obj.([]interface{}) {
		t.Error(subobj.(map[string]interface{}))
	}*/
	//t.Error(obj, err)
	/*n := goyaml.Node{}
	err = goyaml.Unmarshal(c, &n)
	nb, err2 := goyaml.Marshal(n)
	t.Error(string(nb), err, err2)*/
	rn, err := kio.FromBytes(c)
	for _, n := range rn {
		t.Error(n.MustString())
	}
	t.Error(err)
}

func TestBufio(t *testing.T) {
	r := strings.NewReader("foo: bar")
	br := bufio.NewReaderSize(r, 2048)
	c, err := br.Peek(2048)
	t.Error(string(c), err)
}

const fooYAML = `

---

---
baz: 123
foo: bar
bar: true
---
foo: bar
bar: true

`

func TestFoo(t *testing.T) {
	//u, err := url.Parse("file:///foo/bar")
	/*u := &url.URL{
		//Scheme: "file",
		Path: ".",
	}
	t.Error(u, nil, u.RequestURI(), u.Host, u.Scheme)*/

	obj := map[string]interface{}{}

	err := yaml.UnmarshalStrict([]byte(fooYAML), &obj)
	t.Errorf("%+v %v", obj, err)
}

func TestGetMediaTypes(t *testing.T) {
	tests := []struct {
		name           string
		opts           []HeaderOption
		key            string
		wantMediaTypes []string
		wantErr        error
	}{
		{
			name: "multiple keys, and values in one key",
			opts: []HeaderOption{
				WithAccept("application/yaml", "application/xml"),
				WithAccept("application/json"),
				WithAccept("text/html, application/xhtml+xml, application/xml;q=0.9, image/webp, */*;q=0.8"),
			},
			key: AcceptKey,
			wantMediaTypes: []string{
				"application/yaml",
				"application/xml",
				"application/json",
				"text/html",
				"application/xhtml+xml",
				"application/xml",
				"image/webp",
				"*/*",
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := textproto.MIMEHeader{}
			for _, opt := range tt.opts {
				opt.ApplyToHeader(h)
			}
			gotMediaTypes, err := GetMediaTypes(h, tt.key)
			assert.Equal(t, tt.wantMediaTypes, gotMediaTypes)
			assert.ErrorIs(t, err, tt.wantErr)
		})
	}
}
