package slug

import "testing"

func TestMake(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{"simple", "Hello World", "hello-world"},
		{"already slug", "hello-world", "hello-world"},
		{"mixed case and spaces", "  Hello   World  ", "hello-world"},
		{"punctuation", "Hello, World!", "hello-world"},
		{"digits", "Go 101", "go-101"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := Make(tt.in); got != tt.want {
				t.Fatalf("Make(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}

func TestMakeFallsBackToRandom(t *testing.T) {
	for _, in := range []string{"", "   ", "你好世界"} {
		got := Make(in)
		if len(got) != 8 {
			t.Fatalf("Make(%q) = %q, expected 8-char fallback", in, got)
		}
	}
}

func TestValidate(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want bool
	}{
		{"valid", "hello-world", true},
		{"digits", "go-101", true},
		{"empty", "", false},
		{"uppercase", "Hello-World", false},
		{"leading dash", "-hello", false},
		{"trailing dash", "hello-", false},
		{"underscore", "hello_world", false},
		{"space", "hello world", false},
		{"chinese", "你好", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := Validate(tt.in); got != tt.want {
				t.Fatalf("Validate(%q) = %v, want %v", tt.in, got, tt.want)
			}
		})
	}
}
