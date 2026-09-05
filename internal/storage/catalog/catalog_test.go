package catalog

import "testing"

func TestNormalizeName(t *testing.T) {
	cases := []struct {
		name    string
		input   string
		wantErr bool
	}{
		{name: "valid", input: "report.txt"},
		{name: "unicode nfc", input: "caf\u00e9.txt"},
		{name: "empty", input: "", wantErr: true},
		{name: "dot", input: ".", wantErr: true},
		{name: "dotdot", input: "..", wantErr: true},
		{name: "reserved exact", input: "CON", wantErr: true},
		{name: "reserved with extension", input: "CON.txt", wantErr: true},
		{name: "reserved lower", input: "nul.txt", wantErr: true},
		{name: "trailing dot", input: "name.", wantErr: true},
		{name: "trailing space", input: "name ", wantErr: true},
		{name: "slash", input: "a/b", wantErr: true},
		{name: "backslash", input: "a\\b", wantErr: true},
		{name: "colon", input: "a:b", wantErr: true},
		{name: "control char", input: "a\x00b", wantErr: true},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := NormalizeName(tc.input)
			if tc.wantErr && err == nil {
				t.Fatalf("expected error for %q, got nil", tc.input)
			}
			if !tc.wantErr && err != nil {
				t.Fatalf("unexpected error for %q: %v", tc.input, err)
			}
		})
	}
}

func TestNormalizePath(t *testing.T) {
	cases := []struct {
		name    string
		input   string
		wantErr bool
	}{
		{name: "valid relative", input: "docs/report.txt"},
		{name: "single segment", input: "report.txt"},
		{name: "absolute", input: "/etc/passwd", wantErr: true},
		{name: "empty segment", input: "a//b", wantErr: true},
		{name: "dotdot", input: "../etc", wantErr: true},
		{name: "reserved segment", input: "dir/CON.txt", wantErr: true},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := NormalizePath(tc.input)
			if tc.wantErr && err == nil {
				t.Fatalf("expected error for %q, got nil", tc.input)
			}
			if !tc.wantErr && err != nil {
				t.Fatalf("unexpected error for %q: %v", tc.input, err)
			}
		})
	}
}
