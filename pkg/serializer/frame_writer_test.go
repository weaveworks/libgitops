package serializer

import (
	"bytes"
	"testing"
)

func Test_byteWriter_Write(t *testing.T) {
	type fields struct {
		to    []byte
		index int
	}
	type args struct {
		from []byte
	}
	tests := []struct {
		name    string
		fields  fields
		args    args
		wantN   int
		wantErr bool
	}{
		{
			name: "simple case",
			fields: fields{
				to: make([]byte, 50),
			},
			args: args{
				from: []byte("Hello!\nFoobar"),
			},
			wantN:   13,
			wantErr: false,
		},
		{
			name: "target too short",
			fields: fields{
				to: make([]byte, 10),
			},
			args: args{
				from: []byte("Hello!\nFoobar"),
			},
			wantN:   0,
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := &byteWriter{
				to:    tt.fields.to,
				index: tt.fields.index,
			}
			gotN, err := w.Write(tt.args.from)
			if (err != nil) != tt.wantErr {
				t.Errorf("byteWriter.Write() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if gotN != tt.wantN {
				t.Errorf("byteWriter.Write() = %v, want %v", gotN, tt.wantN)
				return
			}
			if !tt.wantErr && !bytes.Equal(tt.fields.to[:gotN], tt.args.from) {
				t.Errorf("byteWriter.Write(): expected fields.to (%s) to equal args.from (%s), but didn't", tt.fields.to[:gotN], tt.args.from)
			}
		})
	}
}
