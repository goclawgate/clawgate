package proxy

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/goclawgate/clawgate/auth"
	"github.com/goclawgate/clawgate/config"
)

// infiniteReader returns 'x' bytes without end.
type infiniteReader struct{}

func (infiniteReader) Read(p []byte) (int, error) {
	for i := range p {
		p[i] = 'x'
	}
	return len(p), nil
}

// TestHandleMessagesRequestTooLarge verifies that a request body
// exceeding maxRequestBytes is rejected with a 413 and the correct
// Anthropic error type ("request_too_large").
func TestHandleMessagesRequestTooLarge(t *testing.T) {
	cfg := &config.Config{AuthMode: "apikey", OpenAIAPIKey: "sk-test"}
	h := NewHandler(cfg)
	mux := http.NewServeMux()
	h.Register(mux)

	// Use a reader that streams just over maxRequestBytes without
	// allocating it all in memory.
	body := io.LimitReader(infiniteReader{}, maxRequestBytes+1)
	req := httptest.NewRequest(http.MethodPost, "/v1/messages", body)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("expected 413, got %d", w.Code)
	}
	var errResp map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &errResp); err != nil {
		t.Fatalf("failed to parse error response: %v", err)
	}
	errObj, ok := errResp["error"].(map[string]interface{})
	if !ok {
		t.Fatalf("missing error object in response: %v", errResp)
	}
	if errObj["type"] != "request_too_large" {
		t.Errorf("expected error type request_too_large, got %v", errObj["type"])
	}
}

// TestHandleTokenCountRequestTooLarge verifies the same 413 behavior
// on the /v1/messages/count_tokens endpoint.
func TestHandleTokenCountRequestTooLarge(t *testing.T) {
	cfg := &config.Config{AuthMode: "apikey", OpenAIAPIKey: "sk-test"}
	h := NewHandler(cfg)
	mux := http.NewServeMux()
	h.Register(mux)

	body := io.LimitReader(infiniteReader{}, maxRequestBytes+1)
	req := httptest.NewRequest(http.MethodPost, "/v1/messages/count_tokens", body)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("expected 413, got %d", w.Code)
	}
	var errResp map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &errResp); err != nil {
		t.Fatalf("failed to parse error response: %v", err)
	}
	errObj, ok := errResp["error"].(map[string]interface{})
	if !ok {
		t.Fatalf("missing error object in response: %v", errResp)
	}
	if errObj["type"] != "request_too_large" {
		t.Errorf("expected error type request_too_large, got %v", errObj["type"])
	}
}

// TestGetTokenSerializesConcurrentRefreshes verifies that when multiple
// goroutines call getToken concurrently while the token is expired,
// only one refresh call is made to the OAuth server.
func TestGetTokenSerializesConcurrentRefreshes(t *testing.T) {
	// Count refresh calls
	var refreshCount int64
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt64(&refreshCount, 1)
		// Simulate a slow refresh
		time.Sleep(50 * time.Millisecond)
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"access_token":"new-access","refresh_token":"new-refresh","expires_in":3600}`))
	}))
	defer srv.Close()

	origIssuer := auth.Issuer
	auth.Issuer = srv.URL
	defer func() { auth.Issuer = origIssuer }()

	// Point token storage at a temp dir so we don't clobber real tokens.
	tmpDir := t.TempDir()
	t.Setenv("HOME", tmpDir)
	t.Setenv("USERPROFILE", tmpDir) // Windows: os.UserHomeDir checks this

	store := &auth.Store{
		Version:        2,
		DefaultAccount: "default",
		Accounts: []auth.StoredAccount{
			{
				Name:         "default",
				AccessToken:  "expired",
				RefreshToken: "refresh-me",
				AccountID:    "acct",
				ExpiresAt:    0, // expired
			},
		},
	}
	if err := auth.SaveStore(store); err != nil {
		t.Fatalf("failed to write expired store: %v", err)
	}

	cfg := &config.Config{AuthMode: "chatgpt", AccountName: "default"}
	h := NewHandler(cfg)

	const goroutines = 10
	var wg sync.WaitGroup
	errs := make([]error, goroutines)
	wg.Add(goroutines)
	for i := 0; i < goroutines; i++ {
		go func(idx int) {
			defer wg.Done()
			ctx := httptest.NewRequest(http.MethodGet, "/", nil).Context()
			_, _, errs[idx] = h.getToken(ctx)
		}(i)
	}
	wg.Wait()

	for i, err := range errs {
		if err != nil {
			t.Errorf("goroutine %d: %v", i, err)
		}
	}

	count := atomic.LoadInt64(&refreshCount)
	if count != 1 {
		t.Errorf("expected exactly 1 refresh call, got %d", count)
	}
}
