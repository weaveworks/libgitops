package frame

import (
	"github.com/weaveworks/libgitops/pkg/content"
	"github.com/weaveworks/libgitops/pkg/frame/sanitize"
	"github.com/weaveworks/libgitops/pkg/util/limitedio"
)

// DefaultMaxFrameCount specifies the default maximum of frames that can be read by a Reader.
const DefaultReadMaxFrameCount = 1024

type singleReaderOptions struct{ SingleOptions }
type singleWriterOptions struct{ SingleOptions }
type readerOptions struct{ Options }
type writerOptions struct{ Options }
type recognizingReaderOptions struct{ RecognizingOptions }
type recognizingWriterOptions struct{ RecognizingOptions }

func defaultSingleReaderOptions() *singleReaderOptions {
	return &singleReaderOptions{
		SingleOptions: SingleOptions{
			MaxFrameSize: limitedio.DefaultMaxReadSize,
			Sanitizer:    sanitize.NewJSONYAML(),
		},
	}
}

func defaultSingleWriterOptions() *singleWriterOptions {
	return &singleWriterOptions{
		SingleOptions: SingleOptions{
			MaxFrameSize: limitedio.Infinite,
			Sanitizer:    sanitize.NewJSONYAML(),
		},
	}
}

func defaultReaderOptions() *readerOptions {
	return &readerOptions{
		Options: Options{
			SingleOptions: defaultSingleReaderOptions().SingleOptions,
			MaxFrameCount: DefaultReadMaxFrameCount,
		},
	}
}

func defaultWriterOptions() *writerOptions {
	return &writerOptions{
		Options: Options{
			SingleOptions: defaultSingleWriterOptions().SingleOptions,
			MaxFrameCount: limitedio.Infinite,
		},
	}
}

func defaultRecognizingReaderOptions() *recognizingReaderOptions {
	return &recognizingReaderOptions{
		RecognizingOptions: RecognizingOptions{
			Options:    defaultReaderOptions().Options,
			Recognizer: content.NewJSONYAMLContentTypeRecognizer(),
		},
	}
}

func defaultRecognizingWriterOptions() *recognizingWriterOptions {
	return &recognizingWriterOptions{
		RecognizingOptions: RecognizingOptions{
			Options:    defaultWriterOptions().Options,
			Recognizer: content.NewJSONYAMLContentTypeRecognizer(),
		},
	}
}

type SingleOptions struct {
	// MaxFrameSize specifies the maximum allowed frame size that can be read and returned.
	// Must be a positive integer. Defaults to DefaultMaxFrameSize. TODO
	MaxFrameSize limitedio.Limit
	// Sanitizer configures the sanitizer that should be used for sanitizing the frames.
	Sanitizer sanitize.Sanitizer
	// TODO: Experiment
	//MetadataOptions []metadata.HeaderOption
}

func (o SingleOptions) applyToSingle(target *SingleOptions) {
	if o.MaxFrameSize != 0 {
		target.MaxFrameSize = o.MaxFrameSize
	}
	if o.Sanitizer != nil {
		target.Sanitizer = o.Sanitizer
	}
	/*if len(o.MetadataOptions) != 0 {
		target.MetadataOptions = append(target.MetadataOptions, o.MetadataOptions...)
	}*/
}

type Options struct {
	SingleOptions

	// MaxFrameCount specifies the maximum amount of successful frames that can be read or written
	// using a Reader or Writer. This means that e.g. empty frames after sanitation are NOT
	// counted as a frame in this context. When reading, there can be a maximum of 10*MaxFrameCount
	// in total (including failed and empty). Must be a positive integer. Defaults: TODO DefaultMaxFrameCount.
	MaxFrameCount limitedio.Limit
}

func (o Options) applyTo(target *Options) {
	if o.MaxFrameCount != 0 {
		target.MaxFrameCount = o.MaxFrameCount
	}
	o.applyToSingle(&target.SingleOptions)
}

type RecognizingOptions struct {
	Options

	Recognizer content.ContentTypeRecognizer
}

func (o RecognizingOptions) applyToRecognizing(target *RecognizingOptions) {
	if o.Recognizer != nil {
		target.Recognizer = o.Recognizer
	}
	o.applyTo(&target.Options)
}

type SingleReaderOption interface {
	ApplyToSingleReader(target *singleReaderOptions)
}

type SingleWriterOption interface {
	ApplyToSingleWriter(target *singleWriterOptions)
}

type ReaderOption interface {
	ApplyToReader(target *readerOptions)
}

type WriterOption interface {
	ApplyToWriter(target *writerOptions)
}

type RecognizingReaderOption interface {
	ApplyToRecognizingReader(target *recognizingReaderOptions)
}

type RecognizingWriterOption interface {
	ApplyToRecognizingWriter(target *recognizingWriterOptions)
}

/*
TODO: Is this needed?
func WithMetadata(mopts ...metadata.HeaderOption) SingleOptions {
	return SingleOptions{MetadataOptions: mopts}
}*/
