package git

import (
	"reflect"
	"testing"
)

func TestParseNumstat(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  []FileChange
	}{
		{
			name:  "empty output",
			input: "",
			want:  nil,
		},
		{
			name:  "single file",
			input: "3\t1\tfoo.go\n",
			want:  []FileChange{{Path: "foo.go", Added: 3, Removed: 1}},
		},
		{
			name:  "multiple files",
			input: "3\t1\tfoo.go\n10\t2\tbar/baz.go\n",
			want:  []FileChange{{Path: "foo.go", Added: 3, Removed: 1}, {Path: "bar/baz.go", Added: 10, Removed: 2}},
		},
		{
			name:  "binary files have no counts",
			input: "5\t0\tfoo.go\n-\t-\timage.png\n",
			want:  []FileChange{{Path: "foo.go", Added: 5}, {Path: "image.png", Binary: true}},
		},
		{
			name:  "renames keep git's path",
			input: "1\t1\tsrc/{old.go => new.go}\n",
			want:  []FileChange{{Path: "src/{old.go => new.go}", Added: 1, Removed: 1}},
		},
		{
			name:  "path with tabs is preserved via SplitN",
			input: "4\t4\tpath\twith\ttabs.go\n",
			want:  []FileChange{{Path: "path\twith\ttabs.go", Added: 4, Removed: 4}},
		},
		{
			name:  "trailing newlines and junk are skipped",
			input: "1\t0\ta.go\n\nnot numstat\n",
			want:  []FileChange{{Path: "a.go", Added: 1}},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := parseNumstat(tt.input); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("parseNumstat(%q) = %+v, want %+v", tt.input, got, tt.want)
			}
		})
	}
}
