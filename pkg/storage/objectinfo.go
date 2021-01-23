package storage

import (
	"github.com/weaveworks/libgitops/pkg/serializer"
	"github.com/weaveworks/libgitops/pkg/storage/core"
)

func NewObjectInfo(ct serializer.ContentType, checksum string, filepath string, id core.UnversionedObjectID) ObjectInfo {
	return &objectInfo{
		ct:       ct,
		checksum: checksum,
		filepath: filepath,
		id:       id,
	}
}

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
