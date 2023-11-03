package frame

import (
	"fmt"

	"github.com/weaveworks/libgitops/pkg/util/limitedio"
	"github.com/weaveworks/libgitops/pkg/util/structerr"
)

// Enforce all struct errors implementing structerr.StructError
var _ structerr.StructError = &FrameCountOverflowError{}

// FrameCountOverflowError is returned when a Reader or Writer would process more
// frames than allowed.
type FrameCountOverflowError struct {
	// +optional
	MaxFrameCount limitedio.Limit
}

func (e *FrameCountOverflowError) Error() string {
	msg := "no more frames can be processed, hit maximum amount"
	if e.MaxFrameCount < 0 {
		msg = fmt.Sprintf("%s: infinity", msg) // this is most likely a programming error
	} else if e.MaxFrameCount > 0 {
		msg = fmt.Sprintf("%s: %d", msg, e.MaxFrameCount)
	}
	return msg
}

func (e *FrameCountOverflowError) Is(target error) bool {
	_, ok := target.(*FrameCountOverflowError)
	return ok
}

// ErrFrameCountOverflow creates a *FrameCountOverflowError
func ErrFrameCountOverflow(maxFrames limitedio.Limit) *FrameCountOverflowError {
	return &FrameCountOverflowError{MaxFrameCount: maxFrames}
}
