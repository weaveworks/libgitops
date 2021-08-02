# Framing

A frame is serialized bytes representing exactly one decodable object, into a Go struct.

The framing package lives in `github.com/weaveworks/libgitops/pkg/frame`, providing YAML and JSON framing by default, but is extensible to other content types as well.

A valid frame should not contain any frame separators (e.g. `---` for YAML), and must not be empty. A frame (and `frame.Reader` or `frame.Writer`) is content-type specific, where the content type is e.g. YAML or JSON.

The source/destination byte stream that is being "framed" by a `frame.Reader` or `frame.Writer` can be for example a file, `/dev/std{in,out,err}`, an HTTP request, or some Go `string`/`[]byte`, for example.

> Note that “frames” and “framer” terminology was borrowed from [`k8s.io/apimachinery`](TODO). Frame maps to the YAML 1.2 spec definition of “documents”, as per below.

## Goals

TODO

## Noteworthy interfaces

TODO

## Default implementations

- `frame.DefaultFactory()` gives you a combined `frame.ReaderFactory` and `frame.WriterFactory` that supports JSON and YAML.

## Examples

### YAML vs JSON frames

This YAML stream contains two frames, i.e. 2 [YAML documents](https://yaml.org/spec/1.2/spec.html#id2800132):

```yaml
---
# Frame 1
foo: bar
bla: true
---
# Frame 2
bar: 123
---
```

The similar list of frames in JSON would be represented as follows:

```json
{
    "foo": "bar",
    "bla": true
}
{
    "bar": 123
}
```

An interesting observation about JSON is that it's "self-framing". The JSON decoder in Go can figure out where an object starts and ends, hence there's no need for extra frame separators, like in YAML.

### Matching a Go struct

"Decodable into a Go struct" means that for the example above, the first frame returned by a framer is:

```yaml
# Frame 1
foo: bar
bla: true
```

```yaml
# Frame 2
bar: 123
```

And this serialized content matches the following Go structs:

```go
type T1 struct {
    Foo string `json:"foo"`
    Bla bool `json:"bla"`
}

type T2 struct {
    Bar int64 `json:"bar"`
}
```

Now, you might ask yourself, that if you look at a generic frame returned from the example above, how do you figure out whether a generic frame should be decoded into `T1` or `T2`, or any other type?

One quick idea would be to annotate the serialized byte representation with some metadata about what content the frame describes. For example, there could be a `kind` field specifying `T1` and `T2` above, respectively.

This is one of the reasons why Kubernetes has [Group, Version and Kinds](https://github.com/kubernetes/community/blob/master/contributors/devel/sig-architecture/api-conventions.md#types-kinds).

But if there's only a `kind` field, it'd be very easy to create naming conflicts, if the whole software ecosystem must agree on or allocate their `kind`s.

For example: There could be `kind: Cluster`, but without any logical grouping, you wouldn't know if it's an etcd, MySQL or Kubernetes cluster that is being referred to.

This is why there exists `group`s in Kubernetes as well. The `apiVersion` field of most Kubernetes-like objects is actually of form: `group/version`. (With exception to `apiVersion: v1` which has `group == ""` (also known as `core`) and `version=="v1"`)

Shortly, the `group` serves as a virtual "namespace" of what the `kind` refers to. `version` specifies the schema of the given object. `version` is very important to allow your schema evolve over time.

For example, imagine some kind of distributed database with the following initial schema

```yaml
apiVersion: my-replicated-db.com/v1alpha1
kind: Database
spec:
  isReplicated: true # A simple boolean telling that the database should be replicated
```

(by convention, versioning starts from `v1alpha1`, that is, "the first alpha release of the first schema version")

Over time, you realize that you actually need to specify _how_ many replicas there should be, so you release `v1alpha2` ("the second alpha release of the first schema version"):

```yaml
apiVersion: my-replicated-db.com/v1alpha2
kind: Database
spec:
  replicas: 3 # A how many replicas should the database use?
```

Later, you realize that there is a need to distinguish between read and write replicas, hence you change the schema once again. But as you feel confident in this design, you upgrade the schema to `v1beta1` ("the first beta release of the first schema version"):

```yaml
apiVersion: my-replicated-db.com/v1beta1
kind: Database
spec:
  replicas: # A how many read/write replicas should the database use?
    read: 3
    write: 1
```

Thanks to specifying the `version` as well, your application can support decoding all three different versions of the objects, as long as you include the corresponding Go structs for all three versions in your Go code.

For now, we don't need to dive into how exactly to decode the frames, but it's important to notice that each frame probably should, for this reason, specify `apiVersion` and `kind`. With this, the example would look like:

```yaml
# Frame 1
apiVersion: foo.com/v1
kind: T1
foo: bar
bla: true
```

```yaml
# Frame 2
apiVersion: foo.com/v1
kind: T2
bar: 123
```

> Note: The struct name and the `kind` necessarily don't need to match, but this is by convention the far most popular way to do it.

### Empty Frames

Empty frames must be ignored, because they are not decodable; they don't map to exactly one Go struct.

To illustrate, the following YAML file contains 2 frames:

```yaml

---
    
---

# Frame 1
apiVersion: foo.com/v1
kind: T1
foo: bar
bla: true

---


---

# Frame 2
apiVersion: foo.com/v1
kind: T2
bar: 123

---
```

TODO: Investigate what happens (or should happen) if there's only comments in a frame. One thing that could be caught in the sanitation process is if the top-level document doesn't any children. However, shall we support retaining that comment-only frame?

### Lists

As per the [Kubernetes API conventions](https://github.com/kubernetes/community/blob/master/contributors/devel/sig-architecture/api-conventions.md#types-kinds), there are "special" kinds with the `List` suffix that contain multiple objects _within the same frame_.

These lists are useful in the REST communication between `kubectl` and the API server, for example. If you want to get a set of same-kind items from the API server, you'd invoke an HTTP request along the lines of:

```http
GET <host>/api/v1/namespaces/default/services
```

and get a response of the form:

```json
{
  "kind": "ServiceList",
  "apiVersion": "v1",
  "metadata": {
    "resourceVersion": "606"
  },
  "items": [
    {
      "metadata": {
        "name": "kubernetes",
        "namespace": "default",
        "labels": {
          "component": "apiserver",
          "provider": "kubernetes"
        }
      },
      "spec": {
        "clusterIP": "10.96.0.1",
      },
      "status": {}
    }
  ]
}
```

(this can be tested with `kubectl get --raw=/api/v1/namespaces/default/services | jq .`)

Why bother returning a `kind: ServiceList` instead of a set of `kind: Service`, separated as JSON frames demonstrated above?

The answer is: a need for returning metadata about the response itself. For example, we can see here that `.metadata.resourceVersion` of the `ServiceList` is set. Other examples of list metadata is pagination headers and information, in case the returned list would be too large to return in only one request.

This does seem specific to just REST communication, and yes, pretty much it is. However, for controllers it presents a nice feature.

The Go struct for typed list (like `ServiceList`), looks something like this:

```go
// From https://github.com/kubernetes/api/blob/v0.21.1/core/v1/types.go#L4423
type ServiceList struct {
    metav1.TypeMeta `json:",inline"`
    metav1.ListMeta `json:"metadata,omitempty"`
    Items []Service `json:"items"`
}
```

If I, as a controller developer, would like to ask for a list of services, what do I do when using e.g. the `controller-runtime` [`Client`](https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.9.2/pkg/client#Reader)?

The answer is, allocate an empty `ServiceList`, pass a pointer of that like follows to get the data:

```go
var svclist []v1.ServiceList
err := client.List(ctx, &svclist)
// svclist.Items is now populated with all returned Services at page 1
...
// If the list of services was larger than the allowed response size, a
// fraction of the services will be returned on the same call. But due to
// that during the first List call, the list's metadata was populated with
// information about what page to ask for next, one can just call List again
// to get the next page.
err := client.List(ctx, &svclist)
// consume more Services at page 2
```

What is useful here, is that `svclist.Items` is of type `[]v1.Service` by definition. There is no need to cast generic objects to Services before using them. Additionally, if the list would contain something else than a `Service`, the decoder would be unable to decode and fail with an error.

These are the existing advantages of using a `List`; these are documented here for additional context.

Because both JSON and YAML support multiple frames, there is technically no direct need to use a `List` in e.g. files checked into Git, if the application reading the byte stream supports framing, that is. If the reading application does not support YAML/JSON framing, using a `List` that can be directly decoded is convenient.

This gives us the conclusion that the following YAML file shall be treated as valid.

```yaml
---
apiVersion: cluster.x-k8s.io/v1alpha4
kind: MachineList
items:
- apiVersion: cluster.x-k8s.io/v1alpha4
  kind: Machine
  spec:
    clusterName: "my-cluster"
- apiVersion: cluster.x-k8s.io/v1alpha4
  kind: Machine
  spec:
    clusterName: "other-cluster"
---
---
apiVersion: cluster.x-k8s.io/v1alpha4
kind: Machine
spec:
  clusterName: "other-cluster"
---
```

How many valid frames are there in the above YAML stream? 2. There's one empty frame that is skipped, one `List` and one "normal" object.

From a framing point of view, we don't know anything about what a `List` is, but it satisfies the contract defined above of being decodable into a single Go struct.

### Limiting Frame Size and Count

If you read a byte stream whose size you're unaware of, e.g. when reading from `/dev/stdin` or an HTTP request, you don't want to open yourself up to a situation where you read garbage forever, sent by a malicious actor. This represents a Denial of Service (DoS) attack vector for your application.

To mitigate that, the builtin `frame.Reader` (and `frame.Writer`, but that's not as important, as the bytes are already in memory) has options to limit the size (byte count) of each frame, and the total frame count, to avoid this situation generally.

The default frame size is 3 Megabytes, which matches the default Kubernetes API server maximum body size.

### Recognizing Readers/Writers

TODO

TODO: We should maybe allow YAML as in "JSON with comments". How to auto-recognize?

```yaml
# This is valid YAML, but invalid JSON, due to these comments
# This works, because YAML is a superset of JSON, and hence one
# can use any valid JSON file, with YAML "extensions" like comments.
{
    # Comment
    "foo": "bar" # Comment
}
```

### Single Readers/Writers

TODO (Any content type)
