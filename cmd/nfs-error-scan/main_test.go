package main

import (
	"strings"
	"testing"
)

func TestHasNFSError(t *testing.T) {
	tokens := prepareTokens(errorTokens)

	cases := []struct {
		name     string
		line     string
		expected bool
	}{
		{name: "nfs error", line: "May 12 08:01:02 node kernel: NFS error: server not responding", expected: true},
		{name: "nfs fail", line: "nfs mount fail due to timeout", expected: true},
		{name: "no nfs", line: "generic error occurred", expected: false},
		{name: "nfs without error token", line: "nfs operation completed successfully", expected: false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := hasNFSError(tc.line, tokens); got != tc.expected {
				t.Fatalf("expected %v, got %v", tc.expected, got)
			}
		})
	}
}

func TestScanReader(t *testing.T) {
	input := "NFS error: access denied\nThis line is fine\nAnother nfs timeout detected\n"
	reader := strings.NewReader(input)

	tokens := prepareTokens(errorTokens)
	matches, err := scanReader("test.log", reader, tokens)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(matches) != 2 {
		t.Fatalf("expected 2 matches, got %d", len(matches))
	}

	if matches[0].line != 1 {
		t.Fatalf("expected first match on line 1, got %d", matches[0].line)
	}

	if matches[1].line != 3 {
		t.Fatalf("expected second match on line 3, got %d", matches[1].line)
	}
}
