package storage

import "github.com/weaveworks/libgitops/pkg/stream"

// ContentTypes describes the connection between
// file extensions and a content types.
var ContentTypes = map[string]stream.ContentType{
	".json": stream.ContentTypeJSON,
	".yaml": stream.ContentTypeYAML,
	".yml":  stream.ContentTypeYAML,
}

func extForContentType(wanted stream.ContentType) string {
	for ext, ct := range ContentTypes {
		if ct == wanted {
			return ext
		}
	}
	return ""
}
