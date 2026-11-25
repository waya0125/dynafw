// SPDX-License-Identifier: MIT
// Copyright (c) 2026 waya0125

package fw

import "testing"

func TestVerifyRejectsInvalid(t *testing.T) {
	if _, err := Verify([]byte("too small")); err == nil {
		t.Fatal("expected error for small file")
	}
	bad := make([]byte, Size105)
	if _, err := Verify(bad); err == nil {
		t.Fatal("expected error for zeroed ARM vector")
	}
}

func TestVerifyAcceptsValidVector(t *testing.T) {
	img := make([]byte, Size105)
	// ARMeb exception vector prefix from DY-UD200 firmware
	copy(img[:8], []byte{0xea, 0x00, 0x00, 0x12, 0xe5, 0x9f, 0xf0, 0x18})
	info, err := Verify(img)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !info.ValidARMeb {
		t.Fatal("expected ValidARMeb=true")
	}
}

func TestPatchRejectsWrongInput(t *testing.T) {
	bad := make([]byte, Size105)
	if _, err := Patch1052To1053(bad); err == nil {
		t.Fatal("expected error for wrong CRC")
	}
}
