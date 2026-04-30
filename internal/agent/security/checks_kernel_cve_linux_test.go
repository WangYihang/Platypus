//go:build linux

package security

import "testing"

// cveAffectsKernel is the central decision function — every kernel CVE
// row goes through it. The test pins the four published-vulnerable
// CVEs against a matrix of kernels covering the boundary cases:
// before-introduction, at-introduction, between-fixes, at-fix, and
// past the upstream cutoff.

func TestCVEAffectsKernel_DirtyPipe(t *testing.T) {
	dp := kernelCVEs[0]
	if dp.cve != "CVE-2022-0847" {
		t.Fatalf("test out of date with table; first entry is %q", dp.cve)
	}
	cases := []struct {
		name          string
		maj, min, pat int
		want          bool
	}{
		{"before introduction (5.7)", 5, 7, 0, false},
		{"at introduction (5.8)", 5, 8, 0, true},
		{"5.10 LTS pre-fix", 5, 10, 50, true},
		{"5.10 LTS at fix", 5, 10, 102, false},
		{"5.10 LTS past fix", 5, 10, 200, false},
		{"5.15 LTS pre-fix", 5, 15, 0, true},
		{"5.15 LTS at fix", 5, 15, 25, false},
		{"5.16 pre-fix", 5, 16, 5, true},
		{"5.16 at fix", 5, 16, 11, false},
		{"6.1 (past upstream cutoff)", 6, 1, 0, false},
		{"7.0 (way past)", 7, 0, 0, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := cveAffectsKernel(dp, c.maj, c.min, c.pat)
			if got != c.want {
				t.Fatalf("cveAffectsKernel(%d.%d.%d) = %v; want %v",
					c.maj, c.min, c.pat, got, c.want)
			}
		})
	}
}

func TestCVEAffectsKernel_OverlayFS(t *testing.T) {
	var ovl kernelCVE
	for _, e := range kernelCVEs {
		if e.cve == "CVE-2023-0386" {
			ovl = e
			break
		}
	}
	if ovl.cve == "" {
		t.Fatal("table missing CVE-2023-0386")
	}
	cases := []struct {
		maj, min, pat int
		want          bool
	}{
		{5, 10, 0, false},  // pre-introduction
		{5, 11, 0, true},   // at introduction
		{5, 15, 90, true},  // 5.15 pre-fix
		{5, 15, 91, false}, // 5.15 at fix
		{6, 1, 0, true},    // 6.1 pre-fix
		{6, 1, 9, false},   // 6.1 at fix
		{6, 2, 0, false},   // upstream cutoff
		{6, 5, 0, false},   // way past
	}
	for _, c := range cases {
		got := cveAffectsKernel(ovl, c.maj, c.min, c.pat)
		if got != c.want {
			t.Errorf("OverlayFS(%d.%d.%d) = %v; want %v",
				c.maj, c.min, c.pat, got, c.want)
		}
	}
}

func TestCVEAffectsKernel_NfTables(t *testing.T) {
	var nf kernelCVE
	for _, e := range kernelCVEs {
		if e.cve == "CVE-2024-1086" {
			nf = e
			break
		}
	}
	if nf.cve == "" {
		t.Fatal("table missing CVE-2024-1086")
	}
	cases := []struct {
		maj, min, pat int
		want          bool
	}{
		{5, 13, 0, false},   // pre-introduction
		{5, 14, 0, true},    // at introduction
		{5, 15, 100, true},  // 5.15 pre-fix
		{5, 15, 149, false}, // 5.15 at fix
		{6, 1, 50, true},    // 6.1 pre-fix
		{6, 1, 76, false},   // 6.1 at fix
		{6, 6, 14, true},    // 6.6 pre-fix
		{6, 6, 15, false},   // 6.6 at fix
		{6, 7, 0, false},    // upstream cutoff
		{6, 8, 0, false},    // way past
	}
	for _, c := range cases {
		got := cveAffectsKernel(nf, c.maj, c.min, c.pat)
		if got != c.want {
			t.Errorf("nf_tables(%d.%d.%d) = %v; want %v",
				c.maj, c.min, c.pat, got, c.want)
		}
	}
}

// CopyFail has no fixedIn table populated yet, so every kernel
// at-or-after the introduction point is reported. Pin that.
func TestCVEAffectsKernel_CopyFail_AlwaysAffectsModern(t *testing.T) {
	var cf kernelCVE
	for _, e := range kernelCVEs {
		if e.cve == "CVE-2026-31431" {
			cf = e
			break
		}
	}
	if cf.cve == "" {
		t.Fatal("table missing CVE-2026-31431")
	}
	if !cveAffectsKernel(cf, 6, 12, 0) {
		t.Errorf("CopyFail should still flag 6.12 (no published fixedIn yet)")
	}
	if !cveAffectsKernel(cf, 6, 18, 8) {
		t.Errorf("CopyFail should flag 6.18.8 (Amazon Linux 2023 confirmed-vulnerable)")
	}
	if cveAffectsKernel(cf, 4, 9, 0) {
		t.Errorf("CopyFail should NOT flag 4.9 (predates introducing commit)")
	}
}
