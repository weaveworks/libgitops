package sanitize

import (
	"context"
	"fmt"
	"log"
	"strings"

	"github.com/pmezard/go-difflib/difflib"
	"sigs.k8s.io/yaml"
)

const testdata = `---
# root

apiVersion: sample.com/v1 # bla
# hello
items:
# moveup
  - item1 # hello
    # bla
  - item2 # hi

kind: MyList # foo
`

type List struct {
	APIVersion string   `json:"apiVersion"`
	Kind       string   `json:"kind"`
	Items      []string `json:"items"`
}

func Example() {
	var list List
	original := []byte(testdata)
	if err := yaml.UnmarshalStrict(original, &list); err != nil {
		log.Fatal(err)
	}
	list.Items = append(list.Items, "item3")

	out, err := yaml.Marshal(list)
	if err != nil {
		log.Fatal(err)
	}
	fmt.Printf("Without sanitation:\n---\n%s---\n", out)
	fmt.Printf("Diff without sanitation:\n---\n%s---\n", doDiff(testdata, string(out)))

	ctx := context.Background()
	sanitized, err := YAML(ctx, out, original)
	if err != nil {
		log.Fatal(err)
	}

	fmt.Printf("With sanitation:\n---\n%s---\n", sanitized)
	fmt.Printf("Diff with sanitation:\n---\n%s---", doDiff(testdata, string(sanitized)))

	// Output:
	// Without sanitation:
	// ---
	// apiVersion: sample.com/v1
	// items:
	// - item1
	// - item2
	// - item3
	// kind: MyList
	// ---
	// Diff without sanitation:
	// ---
	// --- Expected
	// +++ Actual
	// @@ -1,13 +1,7 @@
	// ----
	// -# root
	// +apiVersion: sample.com/v1
	// +items:
	// +- item1
	// +- item2
	// +- item3
	// +kind: MyList
	// -apiVersion: sample.com/v1 # bla
	// -# hello
	// -items:
	// -# moveup
	// -  - item1 # hello
	// -    # bla
	// -  - item2 # hi
	// -
	// -kind: MyList # foo
	// -
	// ---
	// With sanitation:
	// ---
	// # root
	// apiVersion: sample.com/v1 # bla
	// # hello
	// items:
	//   # moveup
	//   - item1 # hello
	//   # bla
	//   - item2 # hi
	//   - item3
	// kind: MyList # foo
	// ---
	// Diff with sanitation:
	// ---
	// --- Expected
	// +++ Actual
	// @@ -1,4 +1,2 @@
	// ----
	//  # root
	// -
	//  apiVersion: sample.com/v1 # bla
	// @@ -6,7 +4,7 @@
	//  items:
	// -# moveup
	// +  # moveup
	//    - item1 # hello
	// -    # bla
	// +  # bla
	//    - item2 # hi
	// -
	// +  - item3
	//  kind: MyList # foo
	// ---
}

func doDiff(a, b string) string {
	diff, _ := difflib.GetUnifiedDiffString(difflib.UnifiedDiff{
		A:        difflib.SplitLines(a),
		B:        difflib.SplitLines(b),
		FromFile: "Expected",
		ToFile:   "Actual",
		Context:  1,
	})
	// Workaround that gofmt is removing the trailing spaces on an "output testing line"
	return strings.ReplaceAll(diff, "\n \n", "\n")
}
