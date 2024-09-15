package hash

import (
	"testing"
)

func TestMD5(t *testing.T) {
	var tests = []struct {
		input string
		want  string
	}{
		{"", "d41d8cd98f00b204e9800998ecf8427e"},
		{"123456", "e10adc3949ba59abbe56e057f20f883e"},
		{"admin", "21232f297a57a5a743894a0e4a801fc3"},
	}
	for _, test := range tests {
		if got := MD5(test.input); got != test.want {
			t.Errorf("MD5(%q) = %v", test.input, got)
		}
	}
}
