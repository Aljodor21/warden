package main

import "testing"

func TestParseEnvValue(t *testing.T) {
	content := "NAS_PASSWORD=secret1\nFILEBROWSER_PASSWORD=abc123xyz\n"
	if got := parseEnvValue(content, "FILEBROWSER_PASSWORD"); got != "abc123xyz" {
		t.Errorf("got %q, want abc123xyz", got)
	}
	if got := parseEnvValue(content, "NAS_PASSWORD"); got != "secret1" {
		t.Errorf("got %q, want secret1", got)
	}
	if got := parseEnvValue(content, "MISSING"); got != "" {
		t.Errorf("got %q, want empty for missing key", got)
	}
}
