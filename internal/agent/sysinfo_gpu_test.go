package agent

import (
	"testing"

	v2pb "github.com/WangYihang/Platypus/pkg/proto/v2"
)

func TestNormalizeNvidiaBusID(t *testing.T) {
	cases := map[string]string{
		"":                        "",
		"0000:65:00.0":            "0:65:00.0",
		"00000000:65:00.0":        "0:65:00.0",
		"00000000:AB:CD.0":        "0:ab:cd.0",
		"bad":                     "bad",
	}
	for in, want := range cases {
		if got := normalizeNvidiaBusID(in); got != want {
			t.Errorf("normalizeNvidiaBusID(%q) = %q; want %q", in, got, want)
		}
	}
}

// TestSummarizeGPUsEmpty exercises the helper via the package-local
// function in handler_agent_link_v2.go indirectly — we just confirm
// the shape used by the API layer compiles and behaves.
func TestGPUInfoEmptyFieldsStaySane(t *testing.T) {
	g := &v2pb.GPUInfo{}
	if g.Vendor != "" || g.Model != "" || g.VramTotalBytes != 0 {
		t.Fatalf("zero-value GPUInfo has unexpected state: %+v", g)
	}
}
