// Metadata contains an interface to work with HTTP-like headers carrying metadata about
// some stream.
package metadata

import (
	"mime"
	"net/textproto"
	"net/url"
	"strconv"
	"strings"
)

/*
	Metadata origin in the system by default:

	stream.FromFile -> stream.Reader
	- X-Content-Location
	- Content-Length

	stream.FromBytes -> stream.Reader
	- Content-Length

	stream.FromString -> stream.Reader
	- Content-Length

	stream.ToFile -> stream.Writer
	- X-Content-Location

	stream.ToBuffer -> stream.Writer

	frame.NewYAMLReader -> frame.Reader
	- Content-Type => YAML

	frame.NewJSONReader -> frame.Reader
	- Content-Type => JSON

	frame.newRecognizingReader -> frame.Reader
	- If Content-Type is set, use that ContentType
	- If X-Content-Location is set, try deduce ContentType from that
	- Peek the stream, and try to deduce the ContentType from that

*/

const (
	// "Known" headers to the system by default, but any other header can also be attached

	XContentLocationKey = "X-Content-Location"

	ContentLengthKey = "Content-Length"
	ContentTypeKey   = "Content-Type"
	AcceptKey        = "Accept"

	// TODO: Add Content-Encoding and Last-Modified?
)

type HeaderOption interface {
	// TODO: Rename to ApplyMetadataHeader?
	ApplyToHeader(target Header)
}

var _ HeaderOption = setHeaderOption{}

func SetOption(k, v string) HeaderOption {
	return setHeaderOption{Key: k, Value: v}
}

func WithContentLength(len int64) HeaderOption {
	return SetOption(ContentLengthKey, strconv.FormatInt(len, 10))
}

func WithContentLocation(loc string) HeaderOption {
	return SetOption(XContentLocationKey, loc)
}

func WithAccept(accepts ...string) HeaderOption {
	return addHeaderOption{Key: AcceptKey, Values: accepts}
}

type setHeaderOption struct{ Key, Value string }

func (o setHeaderOption) ApplyToHeader(target Header) {
	target.Set(o.Key, o.Value)
}

type addHeaderOption struct {
	Key    string
	Values []string
}

func (o addHeaderOption) ApplyToHeader(target Header) {
	for _, val := range o.Values {
		target.Add(o.Key, val)
	}
}

// Make sure the interface is compatible with the targeted textproto.MIMEHeader
var _ Header = textproto.MIMEHeader{}

// Express the string-string map interface of the net/textproto.Header map
type Header interface {
	Add(key, value string)
	Set(key, value string)
	Get(key string) string
	Values(key string) []string
	Del(key string)
}

// TODO: Public or private?

func GetString(m Header, key string) (string, bool) {
	if len(m.Values(key)) == 0 {
		return "", false
	}
	return m.Get(key), true
}

func GetInt64(m Header, key string) (int64, bool) {
	i, err := strconv.ParseInt(m.Get(key), 10, 64)
	if err != nil {
		return 0, false
	}
	return i, true
}

func GetURL(m Header, key string) (*url.URL, bool) {
	str, ok := GetString(m, key)
	if !ok {
		return nil, false
	}
	u, err := url.Parse(str)
	if err != nil {
		return nil, false
	}
	return u, true
}

func GetMediaTypes(m Header, key string) (mediaTypes []string, err error) {
	for _, commaSepVal := range m.Values(key) {
		for _, mediaTypeStr := range strings.Split(commaSepVal, ",") {
			mediaType, _, err := mime.ParseMediaType(mediaTypeStr)
			if err != nil {
				return nil, err
			}
			mediaTypes = append(mediaTypes, mediaType)
		}
	}
	return
}
