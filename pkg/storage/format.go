package storage

import "github.com/weaveworks/libgitops/pkg/content"

// ContentTypes describes the connection between
// file extensions and a content types.
var ContentTypes = map[string]content.ContentType{
	".json": content.ContentTypeJSON,
	".yaml": content.ContentTypeYAML,
	".yml":  content.ContentTypeYAML,
}

func extForContentType(wanted content.ContentType) string {
	for ext, ct := range ContentTypes {
		if ct == wanted {
			return ext
		}
	}
	return ""
}
