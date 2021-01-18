package raw

import (
	"github.com/weaveworks/libgitops/pkg/serializer"
	"github.com/weaveworks/libgitops/pkg/storage/core"
)

var _ ObjectInfo = &objectInfo{}

type objectInfo struct {
	ct       serializer.ContentType
	checksum string
	filepath string
	id       core.UnversionedObjectID
}

func (o *objectInfo) ContentType() serializer.ContentType { return o.ct }
func (o *objectInfo) Checksum() string                    { return o.checksum }
func (o *objectInfo) Path() string                        { return o.filepath }
func (o *objectInfo) ID() core.UnversionedObjectID        { return o.id }
