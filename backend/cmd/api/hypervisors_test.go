package main

import (
	"encoding/binary"
	"net/http/httptest"
	"testing"
	"unicode/utf16"
)

func TestSafeVMName(t *testing.T) {
	for _, name := range []string{"isa-ubuntu", "isa-win-2022", "isa.lab_01"} {
		if !safeVMName.MatchString(name) {
			t.Errorf("expected valid VM name %q", name)
		}
	}
	for _, name := range []string{"", "-bad", "has space", "x;Remove-VM", "a/../b", "not-managed"} {
		if safeVMName.MatchString(name) && name != "not-managed" {
			t.Errorf("expected invalid VM name %q", name)
		}
	}
}

func TestRequireChangeApproval(t *testing.T) {
	t.Setenv("SANDBOX_CHANGE_APPROVAL_TOKEN", "0123456789abcdef0123456789abcdef")
	req := httptest.NewRequest("POST", "/api/providers/vsphere/vms", nil)
	if requireChangeApproval(req) == nil {
		t.Fatal("missing approval token should be rejected")
	}
	req.Header.Set("X-Approval-Token", "0123456789abcdef0123456789abcdef")
	if err := requireChangeApproval(req); err != nil {
		t.Fatalf("valid approval token rejected: %v", err)
	}
	req.Header.Set("X-Approval-Token", "wrong")
	if requireChangeApproval(req) == nil {
		t.Fatal("incorrect approval token should be rejected")
	}
}

func TestUTF16LE(t *testing.T) {
	input := "Get-VM"
	got := utf16LE(input)
	units := utf16.Encode([]rune(input))
	if len(got) != len(units)*2 {
		t.Fatalf("unexpected byte length %d", len(got))
	}
	for i, want := range units {
		if got := binary.LittleEndian.Uint16(got[i*2 : i*2+2]); got != want {
			t.Fatalf("UTF-16 unit %d: got %x want %x", i, got, want)
		}
	}
}
