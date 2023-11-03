package stream

import "go.opentelemetry.io/otel/attribute"

const (
	SpanAttributeKeyByteContent     = "byteContent"
	SpanAttributeKeyByteContentLen  = "byteContentLength"
	SpanAttributeKeyByteContentCap  = "byteContentCapacity"
	SpanAttributeKeyContentMetadata = "contentMetadata"
)

// SpanAttrByteContent registers byteContent and byteContentLength span attributes
// b should be the byte content that has been e.g. read or written in an io operation
func SpanAttrByteContent(b []byte) []attribute.KeyValue {
	return []attribute.KeyValue{
		attribute.String(SpanAttributeKeyByteContent, string(b)),
		attribute.Int64(SpanAttributeKeyByteContentLen, int64(len(b))),
	}
}

// SpanAttrByteContentCap extends SpanAttrByteContent with a capacity argument
// cap should be the capacity of e.g. that read or write, i.e. how much
// could have been read or written.
func SpanAttrByteContentCap(b []byte, cap int) []attribute.KeyValue {
	return append(SpanAttrByteContent(b),
		attribute.Int(SpanAttributeKeyByteContentCap, cap),
	)
}

// TODO: This should be used upstream, too, or not?
func SpanAttrContentMetadata(m Metadata) attribute.KeyValue {
	return attribute.Any(SpanAttributeKeyContentMetadata, m)
}
