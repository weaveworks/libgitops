package tracing

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
)

func Test_tracerName(t *testing.T) {
	tests := []struct {
		obj  interface{}
		want string
	}{
		{"foo", "foo"},
		{trNamed{"bar"}, "bar"},
		{nil, ""},
		{bytes.NewBuffer(nil), "*bytes.Buffer"},
		{os.Stdin, "os.Stdin"},
		{os.Stdout, "os.Stdout"},
		{os.Stderr, "os.Stderr"},
		{io.Discard, "io.Discard"},
	}
	for i, tt := range tests {
		t.Run(fmt.Sprintf("%d", i), func(t *testing.T) {
			assert.Equal(t, tt.want, tracerName(tt.obj))
		})
	}
}

type trNamed struct{ name string }

func (t trNamed) TracerName() string { return t.name }

func Test_funcTracer_fmtSpanName(t *testing.T) {
	tests := []struct {
		tracerName string
		fnName     string
		want       string
	}{
		{tracerName: "Tracer", fnName: "Func", want: "Tracer.Func"},
		{tracerName: "", fnName: "Func", want: "Func"},
		{tracerName: "Tracer", fnName: "", want: "Tracer"},
		{tracerName: "", fnName: "", want: "<unnamed_span>"},
	}
	for i, tt := range tests {
		t.Run(fmt.Sprintf("%d", i), func(t *testing.T) {
			assert.Equal(t, tt.want, funcTracer{name: tt.tracerName}.fmtSpanName(tt.fnName))
		})
	}
}
