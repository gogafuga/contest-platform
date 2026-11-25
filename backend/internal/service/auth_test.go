package service

import (
	"errors"
	"testing"
)

func TestHashTokenShapeAndRoundTrip(t *testing.T) {
	token := "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcd"
	h1 := hashToken(token)
	h2 := hashToken(token)

	if len(h1) != 64 {
		t.Fatalf("hashToken() length = %d, want 64 (sha256 hex)", len(h1))
	}
	if h1 != h2 {
		t.Fatalf("hashToken() is not deterministic: %q != %q", h1, h2)
	}
	if hashToken("different-token") == h1 {
		t.Fatalf("hashToken() collided for different inputs")
	}
}

func TestIssueTokenShape(t *testing.T) {
	a := NewAuth(nil, 0)
	token, err := a.issueTokenRaw()
	if err != nil {
		t.Fatalf("issueTokenRaw() error: %v", err)
	}
	if len(token) != 64 {
		t.Fatalf("token length = %d, want 64 (32 random bytes hex-encoded)", len(token))
	}
}

func TestRegisterValidation(t *testing.T) {
	a := NewAuth(nil, 0)

	if _, _, err := a.Register(nil, "A", "group"); !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("Register() with 1-char name: got err=%v, want ErrInvalidInput", err)
	}
	if _, _, err := a.Register(nil, "Full Name", "  "); !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("Register() with blank group: got err=%v, want ErrInvalidInput", err)
	}
}
