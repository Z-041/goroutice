package pagination

import "testing"

func TestParse(t *testing.T) {
	tests := []struct {
		name     string
		page     string
		size     string
		wantPage int
		wantSize int
	}{
		{"defaults", "", "", 1, 10},
		{"valid", "2", "20", 2, 20},
		{"invalid strings", "abc", "xyz", 1, 10},
		{"non-positive", "0", "-5", 1, 10},
		{"size capped", "3", "999", 3, 100},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			q := Parse(tt.page, tt.size)
			if q.Page != tt.wantPage || q.Size != tt.wantSize {
				t.Fatalf("Parse(%q, %q) = {%d,%d}, want {%d,%d}",
					tt.page, tt.size, q.Page, q.Size, tt.wantPage, tt.wantSize)
			}
		})
	}
}

func TestQueryOffset(t *testing.T) {
	if got := (Query{Page: 3, Size: 20}).Offset(); got != 40 {
		t.Fatalf("Offset() = %d, want 40", got)
	}
}
