/*
Copyright 2015 The Kubernetes Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

// This file provides a means to extract one YAML frame from an io.ReadCloser
//
// This code is (temporarily) forked and derived from
// https://github.com/kubernetes/apimachinery/blob/v0.21.2/pkg/util/yaml/decoder.go#L111
// and will be upstreamed if maintainers allow. The reason for forking this
// small piece of code is two-fold: a) The upstream doesn't allow configuring
// the maximum frame size, but hard-codes it to 5MB and b) for the first
// frame, the "---\n" prefix is returned and would otherwise be unnecessarily
// counted as frame content, when it actually is a frame separator.

package frame

import (
	"bufio"
	"bytes"
	"io"
)

// k8sYAMLReader reads chunks of objects and returns ErrShortBuffer if
// the data is not sufficient.
type k8sYAMLReader struct {
	r         io.ReadCloser
	scanner   *bufio.Scanner
	remaining []byte
}

// newK8sYAMLReader decodes YAML documents from the provided
// stream in chunks by converting each document (as defined by
// the YAML spec) into its own chunk. io.ErrShortBuffer will be
// returned if the entire buffer could not be read to assist
// the caller in framing the chunk.
func newK8sYAMLReader(r io.ReadCloser, maxFrameSize int) io.ReadCloser {
	scanner := bufio.NewScanner(r)
	// the size of initial allocation for buffer 4k
	buf := make([]byte, 4*1024)
	// the maximum size used to buffer a token 5M
	scanner.Buffer(buf, maxFrameSize)
	scanner.Split(splitYAMLDocument)
	return &k8sYAMLReader{
		r:       r,
		scanner: scanner,
	}
}

// Read reads the previous slice into the buffer, or attempts to read
// the next chunk.
// TODO: switch to readline approach.
func (d *k8sYAMLReader) Read(data []byte) (n int, err error) {
	left := len(d.remaining)
	if left == 0 {
		// return the next chunk from the stream
		if !d.scanner.Scan() {
			err := d.scanner.Err()
			if err == nil {
				err = io.EOF
			}
			return 0, err
		}
		out := d.scanner.Bytes()
		// TODO: This could be removed by the sanitation step; we don't have to
		// do it here at this point.
		out = bytes.TrimPrefix(out, []byte("---\n"))
		d.remaining = out
		left = len(out)
	}

	// fits within data
	if left <= len(data) {
		copy(data, d.remaining)
		d.remaining = nil
		return left, nil
	}

	// caller will need to reread
	copy(data, d.remaining[:len(data)])
	d.remaining = d.remaining[len(data):]
	return len(data), io.ErrShortBuffer
}

func (d *k8sYAMLReader) Close() error {
	return d.r.Close()
}

const yamlSeparator = "\n---"

// splitYAMLDocument is a bufio.SplitFunc for splitting YAML streams into individual documents.
func splitYAMLDocument(data []byte, atEOF bool) (advance int, token []byte, err error) {
	if atEOF && len(data) == 0 {
		return 0, nil, nil
	}
	sep := len([]byte(yamlSeparator))
	if i := bytes.Index(data, []byte(yamlSeparator)); i >= 0 {
		// We have a potential document terminator
		i += sep
		after := data[i:]
		if len(after) == 0 {
			// we can't read any more characters
			if atEOF {
				return len(data), data[:len(data)-sep], nil
			}
			return 0, nil, nil
		}
		if j := bytes.IndexByte(after, '\n'); j >= 0 {
			return i + j + 1, data[0 : i-sep], nil
		}
		return 0, nil, nil
	}
	// If we're at EOF, we have a final, non-terminated line. Return it.
	if atEOF {
		return len(data), data, nil
	}
	// Request more data.
	return 0, nil, nil
}
