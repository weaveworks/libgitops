package sanitize

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"strings"

	"github.com/weaveworks/libgitops/pkg/content"
	"github.com/weaveworks/libgitops/pkg/frame/sanitize/comments"
	"k8s.io/utils/pointer"
	"sigs.k8s.io/kustomize/kyaml/kio"
	"sigs.k8s.io/kustomize/kyaml/yaml"
)

// Sanitizer is an interface for sanitizing frames. Note that a sanitizer can only do
// its work correctly if frame actually only contains one frame within.
type Sanitizer interface {
	// Sanitize sanitizes the frame in a standardized way for the given
	// FramingType. If the FramingType isn't known, the Sanitizer can choose between
	// returning an ErrUnsupportedFramingType error or just returning frame, nil unmodified.
	// If ErrUnsupportedFramingType is returned, the consumer won't probably be able to handle
	// other framing types than the default ones, which might not be desired.
	//
	// The returned frame should have len == 0 if it's considered empty.
	Sanitize(ctx context.Context, ct content.ContentType, frame []byte) ([]byte, error)

	content.ContentTypeSupporter
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
		- PreserveOrder (if unset) => preserves the order from the prior if given. no-op if no prior.
		- Alphabetic => sorts all keys alphabetically
		- None => don't preserve order from the prior; no-op
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

func (s *defaultSanitizer) Sanitize(ctx context.Context, ct content.ContentType, frame []byte) ([]byte, error) {
	switch ct {
	case content.ContentTypeYAML:
		return s.handleYAML(ctx, frame)
	case content.ContentTypeJSON:
		return s.handleJSON(frame)
	default:
		// Just passthrough
		return frame, nil
	}
}

func (defaultSanitizer) SupportedContentTypes() content.ContentTypes {
	return []content.ContentType{content.ContentTypeYAML, content.ContentTypeJSON}
}

var ErrTooManyFrames = errors.New("too many frames")

/*
- New policy got applied to all files
- Previously existing policy got applied
*/

// TODO: Make sure maps are alphabetically sorted, or match the prior
// Can e.g. use https://github.com/kubernetes-sigs/kustomize/blob/master/kyaml/order/syncorder.go
func (s *defaultSanitizer) handleYAML(ctx context.Context, frame []byte) ([]byte, error) {
	// Get prior data, if any (from the context), that we'll use to copy comments over and
	// infer the sequence indenting style.
	priorData, hasPriorData := GetPriorData(ctx)

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

	if hasPriorData && s.opts.CopyComments != nil && *s.opts.CopyComments {
		priorNode, err := yaml.Parse(string(priorData))
		if err != nil {
			return nil, err
		}
		// Copy comments over
		if err := comments.CopyComments(priorNode, frameNode, true); err != nil {
			return nil, err
		}
	}

	return yaml.MarshalWithOptions(frameNode.Document(), &yaml.EncoderOptions{
		SeqIndent: s.resolveSeqStyle(frame, priorData, hasPriorData),
	})
}

func (s *defaultSanitizer) resolveSeqStyle(frame, priorData []byte, hasPriorData bool) yaml.SequenceIndentStyle {
	// If specified, use these; can be used as "force-formatting" directives for consistency
	if len(s.opts.ForceSeqIndentStyle) != 0 {
		return s.opts.ForceSeqIndentStyle
	}
	// Otherwise, autodetect the indentation from prior data, if exists, or the current frame
	// If the sequence style cannot be derived; the compact form will be used
	var deriveYAML string
	if hasPriorData {
		deriveYAML = string(priorData)
	} else {
		deriveYAML = string(frame)
	}
	return yaml.SequenceIndentStyle(yaml.DeriveSeqIndentStyle(deriveYAML))
}

// TODO: Maybe use the "Remarshal" property defined here to apply alphabetic order?
// https://stackoverflow.com/questions/18668652/how-to-produce-json-with-sorted-keys-in-go
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

func IfSupported(ctx context.Context, s Sanitizer, ct content.ContentType, frame []byte) ([]byte, error) {
	// If the content type isn't supported, nothing to do
	if s == nil || !s.SupportedContentTypes().Has(ct) {
		return frame, nil
	}
	return s.Sanitize(ctx, ct, frame)
}

func WithPriorData(ctx context.Context, frame []byte) context.Context {
	return context.WithValue(ctx, priorDataKey, frame)
}

func GetPriorData(ctx context.Context) ([]byte, bool) {
	b, ok := ctx.Value(priorDataKey).([]byte)
	return b, ok
}

type priorDataKeyStruct struct{}

var priorDataKey = priorDataKeyStruct{}
