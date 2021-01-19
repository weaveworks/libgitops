package watch

import (
	"fmt"

	"github.com/weaveworks/libgitops/pkg/storage/core"
)

// FileEventType is an enum describing a change in a file's state
type FileEventType byte

const (
	FileEventNone   FileEventType = iota // 0
	FileEventModify                      // 1
	FileEventDelete                      // 2
	FileEventMove                        // 3
)

func (e FileEventType) String() string {
	switch e {
	case 0:
		return "NONE"
	case 1:
		return "MODIFY"
	case 2:
		return "DELETE"
	case 3:
		return "MOVE"
	}

	return "UNKNOWN"
}

// FileEvent describes a file change of a certain kind at a certain
// (relative) path. Often emitted by FileEventsEmitter.
type FileEvent struct {
	Path string
	Type FileEventType
}

// FileEventStream is a channel of FileEvents
type FileEventStream chan *FileEvent

// ObjectEventType is an enum describing a change in an Object's state.
type ObjectEventType byte

var _ fmt.Stringer = ObjectEventType(0)

const (
	ObjectEventNone   ObjectEventType = iota // 0
	ObjectEventCreate                        // 1
	ObjectEventUpdate                        // 2
	ObjectEventDelete                        // 3
	ObjectEventSync                          // 4
)

func (o ObjectEventType) String() string {
	switch o {
	case 0:
		return "NONE"
	case 1:
		return "CREATE"
	case 2:
		return "UPDATE"
	case 3:
		return "DELETE"
	case 4:
		return "SYNC"
	}

	// Should never happen
	return "UNKNOWN"
}

// ObjectEvent describes a change that has been observed
// for the given object with the given ID.
type ObjectEvent struct {
	ID   core.UnversionedObjectID
	Type ObjectEventType
}

// ObjectEventStream is a channel of ObjectEvents
type ObjectEventStream chan *ObjectEvent
