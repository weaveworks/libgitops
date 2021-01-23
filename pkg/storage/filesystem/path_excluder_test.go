package filesystem

import (
	"context"
	"testing"
)

func TestExcludeGitDirectory_ShouldExcludePath(t *testing.T) {
	tests := []struct {
		name string
		path string
		want bool
	}{
		{
			name: "normal",
			path: ".git",
			want: true,
		},
		{
			name: "with relative path",
			path: "./.git",
			want: true,
		},
		{
			name: "with many parents",
			path: "/foo/bar/.git",
			want: true,
		},
		{
			name: "with many children",
			path: ".git/foo/bar/baz",
			want: true,
		},
		{
			name: "with parents and children",
			path: "./foo/bar/.git/baz/bar",
			want: true,
		},
		{
			name: "empty",
			path: "",
			want: false,
		},
		{
			name: "local dir",
			path: ".",
			want: false,
		},
		{
			name: "other prefix",
			path: "foo.git",
			want: false,
		},
		{
			name: "other suffix",
			path: ".gitea",
			want: false,
		},
	}
	e := ExcludeGitDirectory{}
	ctx := context.Background()
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := e.ShouldExcludePath(ctx, nil, tt.path); got != tt.want {
				t.Errorf("ExcludeGitDirectory.ShouldExcludePath() = %v, want %v", got, tt.want)
			}
		})
	}
}
