package str

import (
	"testing"
)

func TestUpperCaseFirstChar(t *testing.T) {
	var tests = []struct {
		input string
		want  string
	}{
		{"", ""},
		{"a", "A"},
		{"admin", "Admin"},
		{",a", ",a"},
		{"1", "1"},
		{"123456", "123456"},
		{"A123456", "A123456"},
	}
	for _, test := range tests {
		if got := UpperCaseFirstChar(test.input); got != test.want {
			t.Errorf("UpperCaseFirstChar(%q) = %v", test.input, got)
		}
	}
}
