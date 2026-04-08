package auth

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// TestRefreshPreservesRefreshTokenWhenOmitted verifies that when the
// OAuth server omits refresh_token from a refresh response (spec-
// compliant: "keep the existing one"), we don't overwrite it with an
// empty string, which would otherwise brick the next refresh cycle.
func TestRefreshPreservesRefreshTokenWhenOmitted(t *testing.T) {
	// Fake /oauth/token that returns an access_token but no refresh_token.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/oauth/token" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"access_token":"new-access","expires_in":3600}`))
	}))
	defer srv.Close()

	origIssuer := Issuer
	Issuer = srv.URL
	defer func() { Issuer = origIssuer }()

	prev := &Token{
		AccessToken:  "old-access",
		RefreshToken: "old-refresh",
		AccountID:    "acct_123",
		ExpiresAt:    0,
	}

	tok, err := Refresh(context.Background(), prev)
	if err != nil {
		t.Fatalf("Refresh returned error: %v", err)
	}
	if tok.AccessToken != "new-access" {
		t.Errorf("AccessToken = %q, want %q", tok.AccessToken, "new-access")
	}
	if tok.RefreshToken != "old-refresh" {
		t.Errorf("RefreshToken = %q, want %q (should fall back to caller's token)", tok.RefreshToken, "old-refresh")
	}
	if tok.AccountID != "acct_123" {
		t.Errorf("AccountID = %q, want %q (should fall back to caller's value)", tok.AccountID, "acct_123")
	}
}

// TestRefreshUsesNewRefreshTokenWhenProvided verifies that a rotated
// refresh token is honored.
func TestRefreshUsesNewRefreshTokenWhenProvided(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"access_token":"new-access","refresh_token":"new-refresh","expires_in":3600}`))
	}))
	defer srv.Close()

	origIssuer := Issuer
	Issuer = srv.URL
	defer func() { Issuer = origIssuer }()

	prev := &Token{
		AccessToken:  "old-access",
		RefreshToken: "old-refresh",
		AccountID:    "acct_123",
	}
	tok, err := Refresh(context.Background(), prev)
	if err != nil {
		t.Fatalf("Refresh returned error: %v", err)
	}
	if tok.RefreshToken != "new-refresh" {
		t.Errorf("RefreshToken = %q, want %q", tok.RefreshToken, "new-refresh")
	}
}

// TestRefreshNilPrev guards against nil dereference when the caller
// hasn't loaded a token.
func TestRefreshNilPrev(t *testing.T) {
	if _, err := Refresh(context.Background(), nil); err == nil {
		t.Error("expected error for nil prev token")
	}
	if _, err := Refresh(context.Background(), &Token{RefreshToken: ""}); err == nil {
		t.Error("expected error for empty refresh token")
	}
}

// TestRefreshHonorsContextCancellation verifies that a cancelled context
// aborts the refresh HTTP call promptly rather than blocking for the
// full authClient.Timeout (30s).
func TestRefreshHonorsContextCancellation(t *testing.T) {
	// Server that takes much longer than the client's context timeout.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(2 * time.Second)
	}))
	defer srv.Close()

	origIssuer := Issuer
	Issuer = srv.URL
	defer func() { Issuer = origIssuer }()

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	prev := &Token{RefreshToken: "tok"}
	_, err := Refresh(ctx, prev)
	if err == nil {
		t.Fatal("expected error from cancelled context")
	}
	if !strings.Contains(err.Error(), "context") {
		t.Errorf("expected context-related error, got: %v", err)
	}
}

// TestRefreshNon200Error verifies that a non-200 response from the
// OAuth server is surfaced as an error containing the status code.
func TestRefreshNon200Error(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(401)
		w.Write([]byte(`{"error":"invalid_grant"}`))
	}))
	defer srv.Close()

	origIssuer := Issuer
	Issuer = srv.URL
	defer func() { Issuer = origIssuer }()

	prev := &Token{RefreshToken: "tok"}
	_, err := Refresh(context.Background(), prev)
	if err == nil {
		t.Fatal("expected error for 401 response")
	}
	if !strings.Contains(err.Error(), "401") {
		t.Errorf("expected error to contain '401', got: %v", err)
	}
}
