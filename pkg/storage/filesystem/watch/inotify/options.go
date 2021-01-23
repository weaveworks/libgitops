package inotify

import (
	"time"

	"github.com/weaveworks/libgitops/pkg/storage/core"
)

// How many inotify events we can buffer before watching is interrupted
const DefaultEventBufferSize int32 = 4096

type FileWatcherOption interface {
	ApplyToFileWatcher(*FileWatcherOptions)
}

var _ FileWatcherOption = &FileWatcherOptions{}

// Options specifies options for the FileWatcher
type FileWatcherOptions struct {
	// PathExcluder specifies what files and directories to ignore
	// Default: ExcludeGitDirectory{}
	PathExcluder core.PathExcluder
	// BatchTimeout specifies the duration to wait after last event
	// before dispatching grouped inotify events
	// Default: 1s
	BatchTimeout time.Duration
	// ContentTyper specifies what content types to recognize.
	// All files for which ContentTyper returns a nil error will
	// be watched.
	// Default: core.DefaultContentTyper
	ContentTyper core.ContentTyper
	// EventBufferSize describes how many inotify events can be buffered
	// before watching is interrupted/delayed.
	// Default: DefaultEventBufferSize
	EventBufferSize int32
}

func (o *FileWatcherOptions) ApplyToFileWatcher(target *FileWatcherOptions) {
	if o.PathExcluder != nil {
		target.PathExcluder = o.PathExcluder
	}
	if o.BatchTimeout != 0 {
		target.BatchTimeout = o.BatchTimeout
	}
	if o.ContentTyper != nil {
		target.ContentTyper = o.ContentTyper
	}
	if o.EventBufferSize != 0 {
		target.EventBufferSize = o.EventBufferSize
	}
}

func (o *FileWatcherOptions) ApplyOptions(opts []FileWatcherOption) *FileWatcherOptions {
	for _, opt := range opts {
		opt.ApplyToFileWatcher(o)
	}
	return o
}

// defaultOptions returns the default options
func defaultOptions() *FileWatcherOptions {
	return &FileWatcherOptions{
		PathExcluder:    core.ExcludeGitDirectory{},
		BatchTimeout:    1 * time.Second,
		ContentTyper:    core.DefaultContentTyper,
		EventBufferSize: DefaultEventBufferSize,
	}
}
