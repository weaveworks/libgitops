package frame

/*var (
	customErr             = errors.New("custom")
	customErrIoReadCloser = errIoReadCloser(customErr)
)*/

/*TODO
func TestNewReader_Unrecognized(t *testing.T) {
	fr := NewReader(FramingType("doesnotexist"), customErrIoReadCloser)
	ctx := context.Background()
	frame, err := fr.ReadFrame(ctx)
	assert.ErrorIs(t, err, ErrUnsupportedFramingType)
	assert.Len(t, frame, 0)
}*/

/*func Test_toReadCloser(t *testing.T) {
	tmp := t.TempDir()
	f, err := os.Create(filepath.Join(tmp, "toReadCloser.txt"))
	require.Nil(t, err)
	defer f.Close()

	tests := []struct {
		name          string
		r             io.Reader
		wantHasCloser bool
	}{
		{
			name:          "*bytes.Reader",
			r:             bytes.NewReader([]byte("foo")),
			wantHasCloser: false,
		},
		{
			name:          "*os.File",
			r:             f,
			wantHasCloser: true,
		},
		{
			name:          "os.Stdout",
			r:             os.Stdout,
			wantHasCloser: false,
		},
		{
			name:          "",
			r:             errIoReadCloser(nil),
			wantHasCloser: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotRc, gotHasCloser := toReadCloser(tt.r)
			wantRc, _ := tt.r.(io.ReadCloser)
			if !tt.wantHasCloser {
				wantRc = io.NopCloser(tt.r)
			}
			assert.Equal(t, wantRc, gotRc)
			assert.Equal(t, tt.wantHasCloser, gotHasCloser)
		})
	}
}*/
