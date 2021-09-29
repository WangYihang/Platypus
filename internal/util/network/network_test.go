package network

import (
	"fmt"
	"testing"
)

func TestParseHostPort(t *testing.T) {
	var tests = []struct {
		input string
		host  string
		port  uint16
		err   error
	}{
		{"127.0.0.1:80", "127.0.0.1", 80, nil},
		{"8.8.8.8:53", "8.8.8.8", 53, nil},
		{"baidu.com:53", "baidu.com", 53, nil},
		{"baidu.com", "", 0, fmt.Errorf("invalid address")},
	}
	for _, test := range tests {
		if hostGot, portGot, errGot := ParseHostPort(test.input); !(hostGot == test.host && portGot == test.port && ((errGot == nil && test.err == nil) || (errGot != nil && test.err != nil && errGot.Error() == test.err.Error()))) {
			t.Errorf("ParseHostPort(%q) = %v %v %v, but %v %v %v wanted", test.input, hostGot, portGot, errGot, test.host, test.port, test.err)
		}
	}
}
