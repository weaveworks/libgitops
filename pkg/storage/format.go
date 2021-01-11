package storage

import "github.com/weaveworks/libgitops/pkg/serializer"

// ContentTypes describes the connection between
// file extensions and a content types.
var ContentTypes = map[string]serializer.ContentType{
	".json": serializer.ContentTypeJSON,
	".yaml": serializer.ContentTypeYAML,
	".yml":  serializer.ContentTypeYAML,
}

var extToContentType = map[serializer.ContentType]string{
	serializer.ContentTypeJSON: ".json",
	serializer.ContentTypeYAML: ".yaml",
}

func extForContentType(wanted serializer.ContentType) string {
	return extToContentType[wanted]
}
