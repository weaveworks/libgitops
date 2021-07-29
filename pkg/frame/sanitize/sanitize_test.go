package sanitize

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/weaveworks/libgitops/pkg/content"
)

func Test_defaultSanitizer_Sanitize(t *testing.T) {
	tests := []struct {
		name     string
		opts     []JSONYAMLOption
		ct       content.ContentType
		prior    string
		frame    string
		want     string
		wantErr  error
		checkErr func(error) bool
	}{
		{
			name:  "passthrough whatever",
			ct:    content.ContentType("unknown"),
			frame: "{randomdata:",
			want:  "{randomdata:",
		},
		{
			name: "default compact",
			ct:   content.ContentTypeJSON,
			frame: `{
				"foo": {
					"bar": "baz"
				}
			}`,
			opts: []JSONYAMLOption{},
			want: `{"foo":{"bar":"baz"}}
`,
		},
		{
			name: "with two spaces",
			ct:   content.ContentTypeJSON,
			frame: `  {  "foo"  : "bar"  }  
`,
			opts: []JSONYAMLOption{WithSpacesIndent(2)},
			want: `{
  "foo": "bar"
}
`,
		},
		{
			name: "with four spaces",
			ct:   content.ContentTypeJSON,
			frame: `  {  "foo"  : {"bar": "baz"}  }  
`,
			opts: []JSONYAMLOption{WithSpacesIndent(4)},
			want: `{
    "foo": {
        "bar": "baz"
    }
}
`,
		},
		{
			name: "with tab indent",
			ct:   content.ContentTypeJSON,
			frame: `  {  "foo"  : {"bar": "baz"}  }  
`,
			opts: []JSONYAMLOption{WithTabsIndent(1)},
			want: `{
	"foo": {
		"bar": "baz"
	}
}
`,
		},
		{
			name:  "with malformed",
			ct:    content.ContentTypeJSON,
			frame: `{"foo":"`,
			opts:  []JSONYAMLOption{WithCompactIndent()},
			checkErr: func(err error) bool {
				_, ok := err.(*json.SyntaxError)
				return ok
			},
		},
		{
			name: "only whitespace",
			ct:   content.ContentTypeJSON,
			frame: `
	
  `,
			want: "",
		},
		{
			name:  "no json",
			ct:    content.ContentTypeJSON,
			frame: "",
			want:  "",
		},
		{
			name: "weird empty formatting",
			ct:   content.ContentTypeYAML,
			frame: `
---
 
       
   `,
			want: "",
		},
		{
			name:  "no yaml",
			ct:    content.ContentTypeYAML,
			frame: "",
			want:  "",
		},
		{
			name: "too many frames",
			ct:   content.ContentTypeYAML,
			frame: `aa: true
---
bb: false
`,
			wantErr: ErrTooManyFrames,
		},
		{
			name: "make sure lists are not expanded",
			ct:   content.ContentTypeYAML,
			frame: `---
kind: List
apiVersion: "v1"
items:
- name: 123
- name: 456
`,
			want: `kind: List
apiVersion: "v1"
items:
- name: 123
- name: 456
`,
		},
		{
			name: "yaml format; don't be confused by the bar commend",
			ct:   content.ContentTypeYAML,
			frame: `---

kind:    List
# foo
apiVersion: "v1"
items:
  # bar
- name: 123
  
`,
			want: `kind: List
# foo
apiVersion: "v1"
items:
# bar
- name: 123
`,
		},
		{
			name: "detect indentation; don't be confused by the bar commend",
			ct:   content.ContentTypeYAML,
			frame: `---

kind:    List
# foo
apiVersion: "v1"
items:
# bar
  - name: 123 
  
`,
			want: `kind: List
# foo
apiVersion: "v1"
items:
  # bar
  - name: 123
`,
		},
		{
			name: "force compact",
			ct:   content.ContentTypeYAML,
			opts: []JSONYAMLOption{WithCompactSeqIndent()},
			frame: `---

kind:    List
# foo
apiVersion: "v1"
items:
  # bar
  - name: 123 
  
`,
			want: `kind: List
# foo
apiVersion: "v1"
items:
# bar
- name: 123
`,
		},
		{
			name: "force wide",
			ct:   content.ContentTypeYAML,
			opts: []JSONYAMLOption{WithWideSeqIndent()},
			frame: `---

kind:    List
# foo
apiVersion: "v1"
items:
# bar
- name: 123 
  
`,
			want: `kind: List
# foo
apiVersion: "v1"
items:
  # bar
  - name: 123
`,
		},
		{
			name: "invalid indentation",
			ct:   content.ContentTypeYAML,
			frame: `---

kind: "foo"
  bar: true`,
			checkErr: func(err error) bool {
				return err.Error() == "yaml: line 1: did not find expected key"
			},
		},
		{
			name: "infer seq style from prior; default is compact",
			ct:   content.ContentTypeYAML,
			opts: []JSONYAMLOption{},
			prior: `# root
# no lists here to look at

kind: List # foo
# bla
apiVersion: v1
`,
			frame: `---
kind:    List
apiVersion: v1
items:
  - item1 # hello
  - item2
`,
			want: `# root
# no lists here to look at

kind: List # foo
# bla
apiVersion: v1
items:
- item1 # hello
- item2
`,
		},
		{
			name: "copy comments; infer seq style from prior",
			ct:   content.ContentTypeYAML,
			opts: []JSONYAMLOption{},
			prior: `# root
# hello

kind: List # foo
# bla
apiVersion: v1
notexist: foo # remember me!

items:
# ignoreme
  - item1 # hello
    # bla
  - item2 # hi
  # after`,
			frame: `---
kind:    List
apiVersion: v1
fruits:
- fruit1
items:
- item1
- item2
- item3
`,
			want: `# root
# hello
# Comments lost during file manipulation:
# Field "notexist": "remember me!"

kind: List # foo
# bla
apiVersion: v1
fruits:
  - fruit1
items:
  # ignoreme
  - item1 # hello
  # bla
  - item2 # hi
  # after

  - item3
`,
		},
		{
			name: "don't copy comments; infer from prior",
			ct:   content.ContentTypeYAML,
			opts: []JSONYAMLOption{WithNoCommentsCopy()},
			prior: `# root
# hello

kind: List # foo
# bla
apiVersion: v1
notexist: foo # remember me!

items:
# ignoreme
- item1 # hello
  # bla
  - item2 # trying to trick the system; but it should make style choice based on item1
  # after`,
			frame: `---
kind:    List
apiVersion: v1
fruits:
- fruit1 # new
items: # new
- item1
- item2
# new
- item3
`,
			want: `kind: List
apiVersion: v1
fruits:
- fruit1 # new
items: # new
- item1
- item2
# new
- item3
`,
		},
		{
			name: "invalid prior",
			ct:   content.ContentTypeYAML,
			prior: `# root
# hello

kind: List # foo
# bla
apiVersion: v1
notexist: foo # remember me!

items:
# ignoreme
  - item1 # hello
  # bla
- item2 # trying to trick the system; but it should make style choice based on item1
  # after`,
			frame: `---
kind:    List
apiVersion: v1
fruits:
- fruit1 # new
items: # new
- item1
- item2
# new
- item3
`,
			checkErr: func(err error) bool {
				return err.Error() == "yaml: line 3: did not find expected key"
			},
		},
		{
			name: "invalid copy comments; change from scalar to mapping node",
			ct:   content.ContentTypeYAML,
			prior: `# root
foo: "bar" # baz`,
			frame: `
foo:
  name: "bar"
`,
			checkErr: func(err error) bool {
				// from sigs.k8s.io/kustomize/kyaml/yaml/fns.go:728
				return err.Error() == `wrong Node Kind for  expected: ScalarNode was MappingNode: value: {name: "bar"}`
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			s := NewJSONYAML(tt.opts...)
			if len(tt.prior) != 0 {
				ctx = WithPriorData(ctx, []byte(tt.prior))
			}
			got, err := s.Sanitize(ctx, tt.ct, []byte(tt.frame))
			assert.Equal(t, tt.want, string(got))
			if tt.checkErr != nil {
				assert.True(t, tt.checkErr(err))
			} else {
				assert.ErrorIs(t, err, tt.wantErr)
			}
		})
	}
}

func TestIfSupported(t *testing.T) {
	ctx := context.Background()
	tests := []struct {
		name    string
		s       Sanitizer
		ct      content.ContentType
		frame   string
		want    string
		wantErr bool
	}{
		{
			name:  "nil sanitizer",
			frame: "foo",
			want:  "foo",
		},
		{
			name:  "unknown content type",
			s:     NewJSONYAML(),
			ct:    content.ContentType("unknown"),
			frame: "foo",
			want:  "foo",
		},
		{
			name:  "sanitize",
			s:     NewJSONYAML(WithCompactIndent()),
			ct:    content.ContentTypeJSON,
			frame: ` { "foo"  : true  }  `,
			want: `{"foo":true}
`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, _ := IfSupported(ctx, tt.s, tt.ct, []byte(tt.frame))
			assert.Equal(t, tt.want, string(got))
		})
	}
}
