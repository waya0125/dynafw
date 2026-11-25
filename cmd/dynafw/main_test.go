// SPDX-License-Identifier: MIT
// Copyright (c) 2026 waya0125

package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRunHelpAndErrors(t *testing.T) {
	var out, errBuf bytes.Buffer
	if err := run([]string{"help"}, strings.NewReader(""), &out, &errBuf); err != nil {
		t.Fatalf("help failed: %v", err)
	}
	if !strings.Contains(out.String(), "dynafw") {
		t.Fatalf("unexpected help output: %q", out.String())
	}

	out.Reset()
	errBuf.Reset()
	if err := run([]string{"unknown"}, strings.NewReader(""), &out, &errBuf); err == nil {
		t.Fatal("expected error for unknown command")
	}
}

func TestVerifyCommandRejectsInvalidFile(t *testing.T) {
	tmp := filepath.Join(t.TempDir(), "bad.bin")
	if err := os.WriteFile(tmp, []byte("invalid"), 0o644); err != nil {
		t.Fatal(err)
	}
	var out, errBuf bytes.Buffer
	if err := run([]string{"verify", tmp}, strings.NewReader(""), &out, &errBuf); err == nil {
		t.Fatal("expected verify error for invalid file")
	}
}
