package frame

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/weaveworks/libgitops/pkg/content"
	"github.com/weaveworks/libgitops/pkg/tracing"
	"github.com/weaveworks/libgitops/pkg/util/compositeio"
	"github.com/weaveworks/libgitops/pkg/util/limitedio"
	"go.opentelemetry.io/otel/trace"
	"go.uber.org/zap/zapcore"
	"k8s.io/utils/pointer"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
)

func init() {
	// Set up the global logger
	log.SetLogger(zap.New(zap.ConsoleEncoder(func(ec *zapcore.EncoderConfig) {
		ec.TimeKey = ""
	}))) // zap.JSONEncoder()

	err := tracing.NewBuilder().
		//RegisterStdoutExporter(stdouttrace.WithWriter(io.Discard)).
		RegisterInsecureJaegerExporter("").
		//WithLogging(true).
		InstallGlobally()
	if err != nil {
		fmt.Printf("failed to install tracing provider: %v\n", err)
		os.Exit(1)
	}
}

// TODO: Make sure that len(frame) == 0 when err != nil for the Writer.

// TODO: Test the output traces more througoutly, when there is SpanProcessor that supports writing
// relevant data to a file, and do matching between spans.

// TODO: Make some 16M (or more) JSON/YAML files and show that these are readable (or not). That's not
// testing a case that already isn't tested by the unit tests below, but would be a good marker that
// it actually solves the right problem.

// TODO: Maybe add some race-condition tests? The centralized place mutexes are used are in
// highlevel{Reader,Writer}, so that'd be the place in that case.

type testcase struct {
	singleReadOpts  []SingleReaderOption
	singleWriteOpts []SingleWriterOption
	// single{Read,Write}Opts are automatically casted to {Reader,Writer}Options if possible
	// and included in readOpts and writeOpts; no need to specify twice
	readOpts  []ReaderOption
	writeOpts []WriterOption
	// {read,write}Opts are automatically casted to Recognizing{Reader,Writer}Options if possible
	// and included in recognizing{Read,Write}Opts; no need to specify twice
	recognizingReadOpts  []RecognizingReaderOption
	recognizingWriteOpts []RecognizingWriterOption

	name     string
	testdata []testdata
	// Reader.ReadFrame will be called len(readResults) times. If a err == nil return is expected, just put
	// nil in the error slice. Similarly for Writer.WriteFrame and writeResults.
	// Note that len(readResults) >= len(frames) and len(writeResults) >= len(frames) must hold.
	// By issuing more reads or writes than there are frames, one can check the error behavior
	readResults  []error
	writeResults []error
	// if closeWriterIdx or closeReaderIdx are non-nil, the Reader/Writer will be closed after the read at
	// that specified index. closeWriterErr and closeReaderErr can be used to check the error returned by
	// the close call.
	closeWriterIdx *int64
	closeWriterErr error
	//expectWriterClosed bool
	closeReaderIdx *int64
	closeReaderErr error

	//expectReaderCloser bool
}

type testdata struct {
	ct                  content.ContentType
	single, recognizing bool
	// frames contain the individual frames of rawData, which in turn is the content of the underlying
	// source/stream. if len(writeResults) == 0, there will be no checking that writing all frames
	// in order will produce the correct rawData. if len(readResults) == 0, there will be no checking
	// that reading rawData will produce the frames string
	rawData string
	frames  []string
}

const (
	yamlSep       = "---\n"
	noNewlineYAML = `foobar: true`
	testYAML      = noNewlineYAML + "\n"
	testYAMLlen   = int64(len(testYAML))
	messyYAMLP1   = `
---

---
` + noNewlineYAML + `
`
	messyYAMLP2 = `

---
---
` + noNewlineYAML + `
---`
	messyYAML = messyYAMLP1 + messyYAMLP2

	testJSON = `{"foo":true}
`
	testJSONlen = int64(len(testJSON))
	testJSON2   = `{"bar":"hello"}
`
	messyJSONP1 = `

` + testJSON + `
`
	messyJSONP2 = `

` + testJSON + `
`
	messyJSON = messyJSONP1 + messyJSONP2

	otherCT       = content.ContentType("other")
	otherFrame    = "('other'; 9)\n('bar'; true)"
	otherFrameLen = int64(len(otherFrame))
)

func TestReader(t *testing.T) {
	// Some tests depend on this
	require.Equal(t, testYAMLlen, testJSONlen)
	NewFactoryTester(t, defaultFactory{}).Test()
	assert.Nil(t, tracing.ForceFlushGlobal(context.Background(), 0))
}

// TODO: Test that closing of Readers and Writers works

var defaultTestCases = []testcase{
	// Roundtrip cases
	{
		name: "simple roundtrip",
		testdata: []testdata{
			{ct: content.ContentTypeYAML, frames: []string{testYAML}, rawData: yamlSep + testYAML},
			{ct: content.ContentTypeJSON, frames: []string{testJSON}, rawData: testJSON},
		},
		writeResults: []error{nil, nil, nil, nil},
		readResults:  []error{nil, io.EOF, io.EOF, io.EOF},
	},

	{
		name: "two-frame roundtrip with closed writer",
		testdata: []testdata{
			{ct: content.ContentTypeYAML, frames: []string{testYAML, testYAML}, rawData: yamlSep + testYAML + yamlSep + testYAML},
			{ct: content.ContentTypeJSON, frames: []string{testJSON, testJSON2}, rawData: testJSON + testJSON2},
		},
		writeResults: []error{nil, nil, nil, nil},
		readResults:  []error{nil, nil, io.EOF, io.EOF},
	},
	// YAML newline addition
	{
		name: "YAML Read: a newline will be added",
		testdata: []testdata{
			{ct: content.ContentTypeYAML, rawData: noNewlineYAML, frames: []string{testYAML}},
		},
		readResults: []error{nil, io.EOF},
	},
	{
		name: "YAML Write: a newline will be added",
		testdata: []testdata{
			{ct: content.ContentTypeYAML, frames: []string{noNewlineYAML}, rawData: yamlSep + testYAML},
		},
		writeResults: []error{nil},
	},
	// Empty frames
	{
		name: "Read: io.EOF when there are no non-empty frames",
		testdata: []testdata{
			{ct: content.ContentTypeYAML, rawData: "---"},
			{ct: content.ContentTypeYAML, rawData: "---\n"},
			{ct: content.ContentTypeJSON, rawData: ""},
			{ct: content.ContentTypeJSON, rawData: "    \n    "},
		},
		readResults: []error{io.EOF},
	},
	{
		name: "Write: Empty sanitized frames aren't written",
		testdata: []testdata{
			{ct: content.ContentTypeYAML, frames: []string{"---", "---\n", " \n--- \n---"}},
			{ct: content.ContentTypeJSON, frames: []string{"", "    \n    ", "  "}},
		},
		writeResults: []error{nil, nil, nil},
	},
	{
		name: "Write: can write empty frames forever without errors",
		testdata: []testdata{
			{ct: content.ContentTypeYAML, frames: []string{testYAML, testYAML}, rawData: yamlSep + testYAML + yamlSep + testYAML},
			{ct: content.ContentTypeJSON, frames: []string{testJSON, testJSON2}, rawData: testJSON + testJSON2},
		},
		writeResults: []error{nil, nil, nil, nil, nil},
		readResults:  []error{nil, nil, io.EOF},
	},
	// Sanitation
	{
		name: "YAML Read: a leading \\n--- will be ignored",
		testdata: []testdata{
			{ct: content.ContentTypeYAML, rawData: "\n" + yamlSep + noNewlineYAML, frames: []string{testYAML}},
		},
		readResults: []error{nil, io.EOF},
	},
	{
		name: "YAML Read: a leading --- will be ignored",
		testdata: []testdata{
			{ct: content.ContentTypeYAML, rawData: yamlSep + noNewlineYAML, frames: []string{testYAML}},
		},
		readResults: []error{nil, io.EOF},
	},
	{
		name: "Read: sanitize messy content",
		testdata: []testdata{
			{ct: content.ContentTypeYAML, rawData: messyYAML, frames: []string{testYAML, testYAML}},
			{ct: content.ContentTypeJSON, rawData: messyJSON, frames: []string{testJSON, testJSON}},
		},
		readResults: []error{nil, nil, io.EOF},
	},
	{
		name: "Write: sanitize messy content",
		testdata: []testdata{
			{ct: content.ContentTypeYAML, frames: []string{messyYAMLP1, messyYAMLP2}, rawData: yamlSep + testYAML + yamlSep + testYAML},
			{ct: content.ContentTypeJSON, frames: []string{messyJSONP1, messyJSONP2}, rawData: testJSON + testJSON},
		},
		writeResults: []error{nil, nil},
	},
	// MaxFrameSize
	{
		name: "Read: the frame size is exactly within bounds, also enforce counter reset",
		testdata: []testdata{
			{ct: content.ContentTypeYAML, rawData: yamlSep + testYAML + yamlSep + testYAML, frames: []string{testYAML, testYAML}},
			{ct: content.ContentTypeJSON, rawData: testJSON + testJSON, frames: []string{testJSON, testJSON}},
		},
		singleReadOpts: []SingleReaderOption{&SingleOptions{MaxFrameSize: limitedio.Limit(testYAMLlen)}},
		readResults:    []error{nil, nil, io.EOF},
	},
	{
		name: "YAML Read: there is a newline before the initial ---, should sanitize",
		testdata: []testdata{
			{ct: content.ContentTypeYAML, rawData: "\n" + yamlSep + testYAML + yamlSep + testYAML, frames: []string{testYAML, testYAML}},
		},
		singleReadOpts: []SingleReaderOption{&SingleOptions{MaxFrameSize: limitedio.Limit(testYAMLlen)}},
		readResults:    []error{nil, nil, io.EOF},
	},
	{
		name: "Read: the frame is out of bounds, on the same line",
		testdata: []testdata{
			{ct: content.ContentTypeYAML, rawData: testYAML},
			{ct: content.ContentTypeJSON, rawData: testJSON},
		},
		singleReadOpts: []SingleReaderOption{&SingleOptions{MaxFrameSize: limitedio.Limit(testYAMLlen - 2)}},
		readResults:    []error{&limitedio.ReadSizeOverflowError{}},
	},
	{
		name: "YAML Read: the frame is out of bounds, but continues on the next line",
		testdata: []testdata{
			{ct: content.ContentTypeYAML, rawData: testYAML + testYAML},
		},
		singleReadOpts: []SingleReaderOption{&SingleOptions{MaxFrameSize: limitedio.Limit(testYAMLlen)}},
		readResults:    []error{&limitedio.ReadSizeOverflowError{}},
	},
	{
		name: "Read: first frame ok, then always frame overflow",
		testdata: []testdata{
			{ct: content.ContentTypeYAML, rawData: testYAML + yamlSep + testYAML + testYAML, frames: []string{testYAML}},
			{ct: content.ContentTypeJSON, rawData: testJSON + testJSON2, frames: []string{testJSON}},
		},
		singleReadOpts: []SingleReaderOption{&SingleOptions{MaxFrameSize: limitedio.Limit(testYAMLlen)}},
		readResults:    []error{nil, &limitedio.ReadSizeOverflowError{}, &limitedio.ReadSizeOverflowError{}, &limitedio.ReadSizeOverflowError{}},
	},
	{
		name: "Write: the second frame is too large, ignore that, but allow writing smaller frames later",
		testdata: []testdata{
			{ct: content.ContentTypeYAML, frames: []string{testYAML, testYAML + testYAML, testYAML}, rawData: yamlSep + testYAML + yamlSep + testYAML},
			{ct: content.ContentTypeJSON, frames: []string{testJSON, testJSON2, testJSON}, rawData: testJSON + testJSON},
		},
		singleWriteOpts: []SingleWriterOption{&SingleOptions{MaxFrameSize: limitedio.Limit(testYAMLlen)}},
		writeResults:    []error{nil, &limitedio.ReadSizeOverflowError{}, nil},
	},
	// TODO: test negative limits too
	{
		name: "first frame ok, then Read => EOF and Write => nil consistently",
		testdata: []testdata{
			{ct: content.ContentTypeYAML, frames: []string{testYAML}, rawData: yamlSep + testYAML},
			{ct: content.ContentTypeJSON, frames: []string{testJSON}, rawData: testJSON},
		},
		readResults:  []error{nil, io.EOF, io.EOF, io.EOF, io.EOF},
		writeResults: []error{nil, nil, nil, nil, nil},
	},
	// MaxFrameCount
	{
		name: "Write: Don't allow writing more than a maximum amount of frames",
		testdata: []testdata{
			{ct: content.ContentTypeYAML, frames: []string{testYAML, testYAML, testYAML}, rawData: yamlSep + testYAML + yamlSep + testYAML},
			{ct: content.ContentTypeJSON, frames: []string{testJSON, testJSON, testJSON}, rawData: testJSON + testJSON},
		},
		writeResults: []error{nil, nil, &FrameCountOverflowError{}, &FrameCountOverflowError{}},
		writeOpts:    []WriterOption{&Options{MaxFrameCount: 2}},
	},
	{
		name: "Read: Don't allow reading more than a maximum amount of successful frames",
		testdata: []testdata{
			{ct: content.ContentTypeYAML,
				rawData: testYAML + yamlSep + testYAML + yamlSep + testYAML,
				frames:  []string{testYAML, testYAML}},
			{ct: content.ContentTypeJSON,
				rawData: testJSON + testJSON + testJSON,
				frames:  []string{testJSON, testJSON}},
		},
		readResults: []error{nil, nil, &FrameCountOverflowError{}, &FrameCountOverflowError{}},
		readOpts:    []ReaderOption{&Options{MaxFrameCount: 2}},
	},
	{
		name: "Read: Don't allow reading more than a maximum amount of successful frames, and 10x in total",
		testdata: []testdata{
			{ct: content.ContentTypeYAML,
				rawData: strings.Repeat("\n"+yamlSep, 10) + testYAML},
		},
		readResults: []error{&FrameCountOverflowError{}, &FrameCountOverflowError{}},
		readOpts:    []ReaderOption{&Options{MaxFrameCount: 1}},
	},
	{
		name: "Read: Allow reading up to the maximum amount of 10x the successful frames count",
		testdata: []testdata{
			{ct: content.ContentTypeYAML,
				rawData: strings.Repeat("\n"+yamlSep, 9) + testYAML + yamlSep + yamlSep, frames: []string{testYAML}},
		},
		readResults: []error{nil, &FrameCountOverflowError{}, &FrameCountOverflowError{}},
		readOpts:    []ReaderOption{&Options{MaxFrameCount: 1}},
	},
	{
		name: "Read: Allow reading exactly that amount of successful frames, if then io.EOF",
		testdata: []testdata{
			{ct: content.ContentTypeYAML,
				rawData: testYAML + yamlSep + testYAML,
				frames:  []string{testYAML, testYAML}},
			{ct: content.ContentTypeJSON,
				rawData: testJSON + testJSON,
				frames:  []string{testJSON, testJSON}},
		},
		readResults: []error{nil, nil, io.EOF, io.EOF},
		readOpts:    []ReaderOption{&Options{MaxFrameCount: 2}},
	},
	// Other Framing Types and Single
	{
		name: "Roundtrip: Allow reading other framing types for single reader, check overflows too",
		testdata: []testdata{
			{ct: otherCT, single: true, rawData: otherFrame, frames: []string{otherFrame}},
		},
		writeResults: []error{nil, &FrameCountOverflowError{}, &FrameCountOverflowError{}, &FrameCountOverflowError{}},
		readResults:  []error{nil, io.EOF, io.EOF, io.EOF},
	},
	{
		name: "Read: other framing type frame size is exactly within bounds",
		testdata: []testdata{
			{ct: otherCT, single: true, rawData: otherFrame, frames: []string{otherFrame}},
		},
		singleReadOpts: []SingleReaderOption{SingleOptions{MaxFrameSize: limitedio.Limit(otherFrameLen)}},
		readResults:    []error{nil, io.EOF},
	},
	{
		name: "Read: other framing type frame size overflow",
		testdata: []testdata{
			{ct: otherCT, single: true, rawData: otherFrame},
		},
		singleReadOpts: []SingleReaderOption{SingleOptions{MaxFrameSize: limitedio.Limit(otherFrameLen - 1)}},
		readResults:    []error{&limitedio.ReadSizeOverflowError{}, io.EOF, io.EOF},
	},
	{
		name: "Write: other framing type frame size overflow",
		testdata: []testdata{
			{ct: otherCT, single: true, frames: []string{otherFrame, otherFrame}},
		},
		singleWriteOpts: []SingleWriterOption{SingleOptions{MaxFrameSize: limitedio.Limit(otherFrameLen - 1)}},
		writeResults:    []error{&limitedio.ReadSizeOverflowError{}, &limitedio.ReadSizeOverflowError{}, nil},
	},
}

func NewFactoryTester(t *testing.T, f Factory) *FactoryTester {
	return &FactoryTester{
		t:       t,
		factory: f,
		cases:   defaultTestCases,
	}
}

type FactoryTester struct {
	t       *testing.T
	factory Factory

	cases []testcase
}

func (h *FactoryTester) Test() {
	for _, c := range h.cases {
		h.t.Run(c.name, func(t *testing.T) {
			h.testRoundtripCase(t, &c)
		})
	}
}

func (h *FactoryTester) testRoundtripCase(t *testing.T, c *testcase) {
	sropt := (&singleReaderOptions{}).applyOptions(c.singleReadOpts)
	swopt := (&singleWriterOptions{}).applyOptions(c.singleWriteOpts)
	ropt := (&readerOptions{}).applyOptions(c.readOpts)
	wopt := (&writerOptions{}).applyOptions(c.writeOpts)

	c.readOpts = append(c.readOpts, sropt)
	c.recognizingReadOpts = append(c.recognizingReadOpts, sropt)
	c.recognizingReadOpts = append(c.recognizingReadOpts, ropt)

	c.writeOpts = append(c.writeOpts, swopt)
	c.recognizingWriteOpts = append(c.recognizingWriteOpts, swopt)
	c.recognizingWriteOpts = append(c.recognizingWriteOpts, wopt)

	ctx := context.Background()
	for i, data := range c.testdata {
		subName := fmt.Sprintf("%d %s", i, data.ct)
		t.Run(subName, func(t *testing.T) {
			tr := tracing.TracerOptions{Name: fmt.Sprintf("%s %s", c.name, subName), UseGlobal: pointer.Bool(true)}
			_ = tr.TraceFunc(ctx, "", func(ctx context.Context, _ trace.Span) error {
				h.testRoundtripCaseContentType(t, ctx, c, &data)
				return nil
			}).Register()
		})
	}
}

func (h *FactoryTester) testRoundtripCaseContentType(t *testing.T, ctx context.Context, c *testcase, d *testdata) {
	var buf bytes.Buffer

	readCloseCounter := &recordingCloser{}
	writeCloseCounter := &recordingCloser{}
	cw := content.NewWriter(compositeio.WriteCloser(&buf, writeCloseCounter))
	cr := content.NewReader(compositeio.ReadCloser(&buf, readCloseCounter))
	var w Writer
	if d.single && d.recognizing {
		panic("cannot be both single and recognizing")
	} else if d.single && !d.recognizing {
		w = h.factory.NewSingleWriter(d.ct, cw, c.singleWriteOpts...)
	} else if !d.single && d.recognizing {
		w = h.factory.NewRecognizingWriter(cw, c.recognizingWriteOpts...)
	} else {
		w = h.factory.NewWriter(d.ct, cw, c.writeOpts...)
	}
	assert.Equalf(t, w.ContentType(), d.ct, "Writer.content.ContentType")

	var r Reader
	if d.single && d.recognizing {
		panic("cannot be both single and recognizing")
	} else if d.single && !d.recognizing {
		r = h.factory.NewSingleReader(d.ct, cr, c.singleReadOpts...)
	} else if !d.single && d.recognizing {
		r = h.factory.NewRecognizingReader(ctx, cr, c.recognizingReadOpts...)
	} else {
		r = h.factory.NewReader(d.ct, cr, c.readOpts...)
	}
	assert.Equalf(t, r.ContentType(), d.ct, "Reader.content.ContentType")

	// Write frames using the writer
	for i, expected := range c.writeResults {
		var frame []byte
		// Only write a frame using the writer if one is supplied
		if i < len(d.frames) {
			frame = []byte(d.frames[i])
		}

		// Write the frame using the writer and check the error
		got := w.WriteFrame(ctx, frame)
		assert.ErrorIsf(t, got, expected, "Writer.WriteFrame err %d", i)

		// If we should close the writer here, do it and check the expected error
		if c.closeWriterIdx != nil && *c.closeWriterIdx == int64(i) {
			assert.ErrorIsf(t, w.Close(ctx), c.closeWriterErr, "Writer.Close err %d", i)
		}
	}

	assert.Equalf(t, 0, writeCloseCounter.count, "Writer should not be closed")

	// Check that the written output was as expected, if writing is enabled
	if len(c.writeResults) != 0 {
		assert.Equalf(t, d.rawData, buf.String(), "Writer Output")
	} else {
		// If writing was not tested, make sure the buffer contains the raw data for reading
		buf = *bytes.NewBufferString(d.rawData)
	}

	// Read frames using the reader
	for i, expected := range c.readResults {
		// Check the expected error
		frame, err := r.ReadFrame(ctx)
		assert.ErrorIsf(t, err, expected, "Reader.ReadFrame err %d", i)
		// Only check the frame content if there's an expected frame
		if i < len(d.frames) {
			assert.Equalf(t, d.frames[i], string(frame), "Reader.ReadFrame frame %d", i)
		}

		// If we should close the reader here, do it and check the expected error
		if c.closeReaderIdx != nil && *c.closeReaderIdx == int64(i) {
			assert.ErrorIsf(t, r.Close(ctx), c.closeReaderErr, "Reader.Close err %d", i)
		}
	}
	assert.Equalf(t, 0, readCloseCounter.count, "Reader should not be closed")
}

type recordingCloser struct {
	count int
}

func (c *recordingCloser) Close() error {
	c.count += 1
	return nil
}
