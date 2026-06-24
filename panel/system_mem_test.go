package main

import "testing"

func TestParseDockerMem(t *testing.T) {
	mib := float64(1 << 20)
	gib := float64(1 << 30)
	cases := map[string]int64{
		"12.34MiB": int64(12.34 * mib),
		"512KiB":   512 << 10,
		"1.2GiB":   int64(1.2 * gib),
		"0B":       0,
		"garbage":  0,
	}
	for in, want := range cases {
		if got := parseDockerMem(in); got != want {
			t.Errorf("parseDockerMem(%q) = %d, want %d", in, got, want)
		}
	}
}
