package frame

var (
	_ SingleReaderOption      = SingleOptions{}
	_ SingleWriterOption      = SingleOptions{}
	_ ReaderOption            = SingleOptions{}
	_ WriterOption            = SingleOptions{}
	_ RecognizingReaderOption = SingleOptions{}
	_ RecognizingWriterOption = SingleOptions{}

	_ SingleReaderOption      = Options{}
	_ SingleWriterOption      = Options{}
	_ ReaderOption            = Options{}
	_ WriterOption            = Options{}
	_ RecognizingReaderOption = Options{}
	_ RecognizingWriterOption = Options{}

	_ SingleReaderOption      = RecognizingOptions{}
	_ SingleWriterOption      = RecognizingOptions{}
	_ ReaderOption            = RecognizingOptions{}
	_ WriterOption            = RecognizingOptions{}
	_ RecognizingReaderOption = RecognizingOptions{}
	_ RecognizingWriterOption = RecognizingOptions{}
)

func (o SingleOptions) ApplyToSingleReader(target *singleReaderOptions) {
	o.applyToSingle(&target.SingleOptions)
}

func (o SingleOptions) ApplyToSingleWriter(target *singleWriterOptions) {
	o.applyToSingle(&target.SingleOptions)
}

func (o SingleOptions) ApplyToReader(target *readerOptions) {
	o.applyToSingle(&target.SingleOptions)
}

func (o SingleOptions) ApplyToWriter(target *writerOptions) {
	o.applyToSingle(&target.SingleOptions)
}

func (o SingleOptions) ApplyToRecognizingReader(target *recognizingReaderOptions) {
	o.applyToSingle(&target.SingleOptions)
}

func (o SingleOptions) ApplyToRecognizingWriter(target *recognizingWriterOptions) {
	o.applyToSingle(&target.SingleOptions)
}

func (o Options) ApplyToReader(target *readerOptions) {
	o.applyTo(&target.Options)
}

func (o Options) ApplyToWriter(target *writerOptions) {
	o.applyTo(&target.Options)
}

func (o Options) ApplyToRecognizingReader(target *recognizingReaderOptions) {
	o.applyTo(&target.Options)
}

func (o Options) ApplyToRecognizingWriter(target *recognizingWriterOptions) {
	o.applyTo(&target.Options)
}

func (o RecognizingOptions) ApplyToRecognizingReader(target *recognizingReaderOptions) {
	o.applyToRecognizing(&target.RecognizingOptions)
}

func (o RecognizingOptions) ApplyToRecognizingWriter(target *recognizingWriterOptions) {
	o.applyToRecognizing(&target.RecognizingOptions)
}

func (o *singleReaderOptions) applyOptions(opts []SingleReaderOption) *singleReaderOptions {
	for _, opt := range opts {
		opt.ApplyToSingleReader(o)
	}
	return o
}

func (o *singleWriterOptions) applyOptions(opts []SingleWriterOption) *singleWriterOptions {
	for _, opt := range opts {
		opt.ApplyToSingleWriter(o)
	}
	return o
}

func (o *readerOptions) applyOptions(opts []ReaderOption) *readerOptions {
	for _, opt := range opts {
		opt.ApplyToReader(o)
	}
	return o
}

func (o *writerOptions) applyOptions(opts []WriterOption) *writerOptions {
	for _, opt := range opts {
		opt.ApplyToWriter(o)
	}
	return o
}

func (o *recognizingReaderOptions) applyOptions(opts []RecognizingReaderOption) *recognizingReaderOptions {
	for _, opt := range opts {
		opt.ApplyToRecognizingReader(o)
	}
	return o
}

func (o *recognizingWriterOptions) applyOptions(opts []RecognizingWriterOption) *recognizingWriterOptions {
	for _, opt := range opts {
		opt.ApplyToRecognizingWriter(o)
	}
	return o
}
