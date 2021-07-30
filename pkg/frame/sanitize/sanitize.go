package sanitize

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"

	"github.com/weaveworks/libgitops/pkg/frame/sanitize/comments"
	"github.com/weaveworks/libgitops/pkg/stream"
	"k8s.io/utils/pointer"
	"sigs.k8s.io/kustomize/kyaml/kio"
	"sigs.k8s.io/kustomize/kyaml/yaml"
)

// JSON sanitizes JSON data in "current" using the NewJSONYAML() sanitizer with the given
// options. Optionally, "original" data can be used to preserve earlier styles.
func JSON(ctx context.Context, current, original []byte, opts ...JSONYAMLOption) ([]byte, error) {
	return Sanitize(ctx, NewJSONYAML(opts...), stream.ContentTypeJSON, current, original)
}

// YAML sanitizes YAML data in "current" using the NewJSONYAML() sanitizer with the given
// options. Optionally, "original" data can be used to preserve earlier styles, e.g. copy
// over comments and remember the sequence indentation style.
func YAML(ctx context.Context, current, original []byte, opts ...JSONYAMLOption) ([]byte, error) {
	return Sanitize(ctx, NewJSONYAML(opts...), stream.ContentTypeYAML, current, original)
}

// Sanitize sanitizes the "current" frame using the Sanitizer s, for the given ContentType if supported.
// If original is non-nil, it'll be used to merge the "current" frame with information from the original,
// for YAML this e.g. means copying comments and remembering the sequence indentation style.
func Sanitize(ctx context.Context, s Sanitizer, ct stream.ContentType, current, original []byte) ([]byte, error) {
	if original != nil {
		ctx = WithOriginalData(ctx, original)
	}
	return IfSupported(ctx, s, ct, current)
}

// IfSupported calls the Sanitizer.Sanitize function using the given Sanitizer if the content type
// is supported. If the content type is not supported, the frame is returned as-is, with no error.
func IfSupported(ctx context.Context, s Sanitizer, ct stream.ContentType, frame []byte) ([]byte, error) {
	// If the content type isn't supported, nothing to do
	if s == nil || !s.SupportedContentTypes().Has(ct) {
		return frame, nil
	}
	return s.Sanitize(ctx, ct, frame)
}

// WithOriginalData registers the given frame with the context such that the frame can be used
// as "original data" when sanitizing. Prior data can be used to copy over YAML comments
// automatically from the original data, remember the key order, sequence indentation level, etc.
func WithOriginalData(ctx context.Context, original []byte) context.Context {
	return context.WithValue(ctx, originalDataKey, original)
}

// GetOriginalData retrieves the original data frame, if any, set using WithOriginalData.
func GetOriginalData(ctx context.Context) ([]byte, bool) {
	b, ok := ctx.Value(originalDataKey).([]byte)
	return b, ok
}

// ErrTooManyFrames is returned if more than one frame is given to the Sanitizer
const ErrTooManyFrames = strConstError("sanitizing multiple frames at once not supported")

type strConstError string

func (s strConstError) Error() string { return string(s) }

type originalDataKeyStruct struct{}

var originalDataKey = originalDataKeyStruct{}

// Sanitizer is an interface for sanitizing frames. Note that a sanitizer can only do
// its work correctly if only one single frame is given at a time. To chop a byte stream
// into frames, see the pkg/frame package.
type Sanitizer interface {
	// Sanitize sanitizes the frame in a standardized way for the given
	// stream.ContentType. If the stream.ContentType isn't known, the Sanitizer should
	// return stream.UnsupportedContentTypeError. The consumer can use IfSupported() to
	// just skip sanitation if the content type is not supported. If multiple frames are
	// given, ErrTooManyFrames can be returned.
	//
	// The returned frame should have len == 0 if it's considered empty.
	Sanitize(ctx context.Context, ct stream.ContentType, frame []byte) ([]byte, error)

	// The Sanitizer supports sanitizing one or many content types
	stream.ContentTypeSupporter
}

// defaultSanitizer implements frame sanitation for JSON and YAML.
//
// For YAML it removes unnecessary "---" separators, whitespace and newlines.
// The YAML frame always ends with a newline, unless the sanitized YAML was an empty string, in which
// case an empty string with len == 0 will be returned.
//
// For JSON it sanitizes the JSON frame by removing unnecessary spaces and newlines around it.
func NewJSONYAML(opts ...JSONYAMLOption) Sanitizer {
	return &defaultSanitizer{defaultJSONYAMLOptions().applyOptions(opts)}
}

func WithCompactIndent() JSONYAMLOption {
	return WithSpacesIndent(0)
}

func WithSpacesIndent(spaces uint8) JSONYAMLOption {
	i := strings.Repeat(" ", int(spaces))
	return &jsonYAMLOptions{Indentation: &i}
}

func WithTabsIndent(tabs uint8) JSONYAMLOption {
	i := strings.Repeat("\t", int(tabs))
	return &jsonYAMLOptions{Indentation: &i}
}

func WithCompactSeqIndent() JSONYAMLOption {
	return &jsonYAMLOptions{ForceSeqIndentStyle: yaml.CompactSequenceStyle}
}

func WithWideSeqIndent() JSONYAMLOption {
	return &jsonYAMLOptions{ForceSeqIndentStyle: yaml.WideSequenceStyle}
}

func WithNoCommentsCopy() JSONYAMLOption {
	return &jsonYAMLOptions{CopyComments: pointer.Bool(false)}
}

type JSONYAMLOption interface {
	applyToJSONYAML(*jsonYAMLOptions)
}

type jsonYAMLOptions struct {
	// Only applicable to JSON at the moment; YAML indentation config not supported
	Indentation *string
	// Only applicable to YAML; either yaml.CompactSequenceStyle or yaml.WideSequenceStyle
	ForceSeqIndentStyle yaml.SequenceIndentStyle
	// Only applicable to YAML; JSON doesn't support comments
	CopyComments *bool
	/*
		TODO: ForceMapKeyOrder that can either be
		- PreserveOrder (default) => preserves the order from the original if given. no-op if no original.
		- Alphabetic => sorts all keys alphabetically
		- None => don't preserve order from the original; no-op
	*/
}

func defaultJSONYAMLOptions() *jsonYAMLOptions {
	return (&jsonYAMLOptions{
		Indentation:  pointer.String(""),
		CopyComments: pointer.Bool(true),
	})
}

func (o *jsonYAMLOptions) applyToJSONYAML(target *jsonYAMLOptions) {
	if o.Indentation != nil {
		target.Indentation = o.Indentation
	}
	if len(o.ForceSeqIndentStyle) != 0 {
		target.ForceSeqIndentStyle = o.ForceSeqIndentStyle
	}
	if o.CopyComments != nil {
		target.CopyComments = o.CopyComments
	}
}

func (o *jsonYAMLOptions) applyOptions(opts []JSONYAMLOption) *jsonYAMLOptions {
	for _, opt := range opts {
		opt.applyToJSONYAML(o)
	}
	return o
}

type defaultSanitizer struct {
	opts *jsonYAMLOptions
}

func (s *defaultSanitizer) Sanitize(ctx context.Context, ct stream.ContentType, frame []byte) ([]byte, error) {
	switch ct {
	case stream.ContentTypeYAML:
		return s.handleYAML(ctx, frame)
	case stream.ContentTypeJSON:
		return s.handleJSON(frame)
	default:
		// Just passthrough
		return frame, nil
	}
}

func (defaultSanitizer) SupportedContentTypes() stream.ContentTypes {
	return []stream.ContentType{stream.ContentTypeYAML, stream.ContentTypeJSON}
}

func (s *defaultSanitizer) handleYAML(ctx context.Context, frame []byte) ([]byte, error) {
	// Get original data, if any (from the context), that we'll use to copy comments over and
	// infer the sequence indenting style.
	originalData, hasOriginalData := GetOriginalData(ctx)

	// Parse the current node
	frameNodes, err := (&kio.ByteReader{
		Reader:                bytes.NewReader(append([]byte{'\n'}, frame...)),
		DisableUnwrapping:     true,
		OmitReaderAnnotations: true,
	}).Read()
	if err != nil {
		return nil, err
	}
	if len(frameNodes) == 0 {
		return []byte{}, nil
	} else if len(frameNodes) != 1 {
		return nil, ErrTooManyFrames
	}
	frameNode := frameNodes[0]

	if hasOriginalData && s.opts.CopyComments != nil && *s.opts.CopyComments {
		originalNode, err := yaml.Parse(string(originalData))
		if err != nil {
			return nil, err
		}
		// Copy comments over
		if err := comments.CopyComments(originalNode, frameNode, true); err != nil {
			return nil, err
		}
	}

	return yaml.MarshalWithOptions(frameNode.Document(), &yaml.EncoderOptions{
		SeqIndent: s.resolveSeqStyle(frame, originalData, hasOriginalData),
	})
}

func (s *defaultSanitizer) resolveSeqStyle(frame, originalData []byte, hasOriginalData bool) yaml.SequenceIndentStyle {
	// If specified, use these; can be used as "force-formatting" directives for consistency
	if len(s.opts.ForceSeqIndentStyle) != 0 {
		return s.opts.ForceSeqIndentStyle
	}
	// Otherwise, autodetect the indentation from original data, if exists, or the current frame
	// If the sequence style cannot be derived; the compact form will be used
	var deriveYAML string
	if hasOriginalData {
		deriveYAML = string(originalData)
	} else {
		deriveYAML = string(frame)
	}
	return yaml.SequenceIndentStyle(yaml.DeriveSeqIndentStyle(deriveYAML))
}

func (s *defaultSanitizer) handleJSON(frame []byte) ([]byte, error) {
	// If it's all whitespace, just return an empty byte array, no actual content here
	if len(bytes.TrimSpace(frame)) == 0 {
		return []byte{}, nil
	}
	var buf bytes.Buffer
	var err error
	if s.opts.Indentation == nil || len(*s.opts.Indentation) == 0 {
		err = json.Compact(&buf, frame)
	} else {
		err = json.Indent(&buf, frame, "", *s.opts.Indentation)
	}
	if err != nil {
		return nil, err
	}
	// Trim all other spaces than an ending newline
	return append(bytes.TrimSpace(buf.Bytes()), '\n'), nil
}
