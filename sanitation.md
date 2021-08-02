# Frame Sanitation

The frame sanitation package that lives in `github.com/weaveworks/libgitops/pkg/frame/sanitation` takes care of formatting frames in a user-configurable and content-type-specific way.

This is useful, for example, when one would like to standardize the formatting of YAML and/or JSON in a Git repository.

## Goals

- Provide a way to, in a content-type specific way, set a "default" formatting (Similar purpose as `gofmt` and `rustfmt`)
- Minimize textual diffs when updating an object (e.g. writing back to git)
- Allow the user to specifically choose formatting options like spacing, field ordering
- Allow retaining auxiliary metadata in the frame, e.g. YAML comments

## Default implementations

- `sanitation.NewJSONYAML()` supports JSON and YAML with the following options:
  - TODO

## Examples

### Minimizing YAML diffs

Take this valid, but messy YAML file as an example of what a user might store in Git:

"YAML File A":

```yaml
---
# root

apiVersion: sample.com/v1 # bla
# hello
items:
# moveup
  - item1 # hello
    # bla
  - item2 # hi

kind: MyList # foo

```

Say that you want to append a `item-3` string to the `items` list. You do a `yaml.Unmarshal` and `yaml.Marshal` using your favorite library, and this is what you'll get:

"YAML File B":

```yaml
apiVersion: sample.com/v1
items:
- item1
- item2
- item3
kind: MyList
```

That's nice and all, it's semantically the right content. However, it's lost all structure from the original YAML document, and the diff is huge and hard to understand:

```diff
--- Expected
+++ Actual
@@ -1,13 +1,7 @@
----
-# root
+apiVersion: sample.com/v1
+items:
+- item1
+- item2
+- item3
+kind: MyList
-apiVersion: sample.com/v1 # bla
-# hello
-items:
-# moveup
-  - item1 # hello
-    # bla
-  - item2 # hi
-
-kind: MyList # foo
-
```

However, if the user calls `sanitize.Sanitize` and gives "YAML File A" as the "original" document and gives "YAML File B" as the "current" document, the JSON/YAML sanitizer will merge these as follows:

```yaml
# root
apiVersion: sample.com/v1 # bla
# hello
items:
  # moveup
  - item1 # hello
  # bla
  - item2 # hi
  - item3
kind: MyList # foo
```

With the diff:

```diff
--- Expected
+++ Actual
@@ -1,4 +1,2 @@
----
 # root
-
 apiVersion: sample.com/v1 # bla
@@ -6,7 +4,7 @@
 items:
-# moveup
+  # moveup
   - item1 # hello
-    # bla
+  # bla
   - item2 # hi
-
+  - item3
 kind: MyList # foo
```

Quite a difference! We can see that the

- Comments from the original document are preserved
  - This is achieved by walking the YAML nodes in the "original" document, and the "current" document. Whenever a comment is found in the "original" document, it is copied over to the "current".
  - 
- Comments are now aligned with the default indentation at that context
  - As per the [YAML 1.2 spec](https://yaml.org/spec/1.2/spec.html#id2767100) "comments are not associated with a particular node".
  - In practice, though, [gopkg.in/yaml.v3
](https://pkg.go.dev/gopkg.in/yaml.v3) **does attach** comments to YAML nodes. Arguably, this is also what users do expect.
  - Hence, what is happening when sanitizing this document is that all comments line up on the same indentation as it's context.
- The unnecessary `---` separator has been removed
  - Frame separators should not be part of the frame
  - Framing is handled by the [framer](framing.md)
- The list indentation is preserved
  - That is, the list items of `items` are indented like in the original A document, but unlike current B
- Unnecessary newlines are removed

TODO: Investigate what happens to comments when you prepend an item to a list.
