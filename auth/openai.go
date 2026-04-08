package auth

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const (
	ClientID      = "app_EMoamEEZ73f0CkXaXp7hrann"
	CodexEndpoint = "https://chatgpt.com/backend-api/codex/responses"
	PollMarginMs  = 3000
	Version       = "1.2.0"

	// tokenExpiryBuffer is the window (in seconds) before the true
	// token expiry where we proactively refresh. Five minutes gives
	// ample time for the refresh round-trip to complete before the
	// access token actually lapses.
	tokenExpiryBuffer = 300
)

// Issuer is the OAuth token/device-auth endpoint base. It is a var
// (not a const) so tests can point it at a local httptest.Server.
var Issuer = "https://auth.openai.com"

// authClient is used for all OAuth HTTP calls so they time out instead
// of blocking forever when the auth server is unresponsive.
var authClient = &http.Client{Timeout: 30 * time.Second}

// postForm sends an application/x-www-form-urlencoded POST using the
// given context, allowing the caller's cancellation to propagate.
func postForm(ctx context.Context, url string, data url.Values) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, strings.NewReader(data.Encode()))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	return authClient.Do(req)
}

// postJSON sends an application/json POST using the given context.
func postJSON(ctx context.Context, url string, body string) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, strings.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	return authClient.Do(req)
}

// Token holds persisted OAuth tokens.
type Token struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	AccountID    string `json:"account_id,omitempty"`
	ExpiresAt    int64  `json:"expires_at"`
}

// IsExpired checks if the access token needs refresh.
func (t *Token) IsExpired() bool {
	return time.Now().Unix() > t.ExpiresAt-tokenExpiryBuffer
}

// ── Device Flow ──────────────────────────────────────────────────────

type deviceCodeResp struct {
	DeviceAuthID string `json:"device_auth_id"`
	UserCode     string `json:"user_code"`
	Interval     string `json:"interval"`
}

type deviceTokenResp struct {
	AuthorizationCode string `json:"authorization_code"`
	CodeVerifier      string `json:"code_verifier"`
}

type oauthTokenResp struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	IDToken      string `json:"id_token"`
	ExpiresIn    int    `json:"expires_in"`
}

// Login performs the OpenAI Codex headless device flow.
func Login() (*Token, error) {
	fmt.Println("\n🔐 ChatGPT Login (Codex)")
	fmt.Println("───────────────────────────")

	// Step 1: Request device code
	body := fmt.Sprintf(`{"client_id":"%s"}`, ClientID)
	resp, err := postJSON(context.Background(), Issuer+"/api/accounts/deviceauth/usercode", body)
	if err != nil {
		return nil, fmt.Errorf("device code request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		b, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("device code failed (%d): %s", resp.StatusCode, string(b))
	}

	var device deviceCodeResp
	if err := json.NewDecoder(resp.Body).Decode(&device); err != nil {
		return nil, fmt.Errorf("parse device response: %w", err)
	}

	// Step 2: Show instructions
	fmt.Printf("\n  1. Open:  \033[1;36m%s/codex/device\033[0m\n", Issuer)
	fmt.Printf("  2. Enter: \033[1;33m%s\033[0m\n\n", device.UserCode)
	fmt.Println("  Waiting for authorization...")

	// Step 3: Poll for token
	intervalSec := 5
	if device.Interval != "" {
		fmt.Sscanf(device.Interval, "%d", &intervalSec)
		if intervalSec < 1 {
			intervalSec = 5
		}
	}
	interval := time.Duration(intervalSec)*time.Second + time.Duration(PollMarginMs)*time.Millisecond

	const deviceFlowTimeout = 5 * time.Minute
	const maxConsecutiveErrors = 5

	deadline := time.Now().Add(deviceFlowTimeout)
	consecutiveErrors := 0
	for time.Now().Before(deadline) {
		time.Sleep(interval)

		tokenResp, status, err := pollDeviceToken(device.DeviceAuthID, device.UserCode)
		if err != nil {
			consecutiveErrors++
			fmt.Printf("  ⚠️  Poll error: %v (retrying...)\n", err)
			if consecutiveErrors >= maxConsecutiveErrors {
				return nil, fmt.Errorf("device flow aborted after %d consecutive poll errors: %w", consecutiveErrors, err)
			}
			continue
		}
		consecutiveErrors = 0

		if status == 200 && tokenResp != nil {
			// Step 4: Exchange code for tokens
			tokens, err := exchangeCode(context.Background(), tokenResp.AuthorizationCode, tokenResp.CodeVerifier)
			if err != nil {
				return nil, fmt.Errorf("token exchange failed: %w", err)
			}

			accountID := extractAccountID(tokens.IDToken)
			if accountID == "" {
				accountID = extractAccountID(tokens.AccessToken)
			}

			expiresIn := tokens.ExpiresIn
			if expiresIn == 0 {
				expiresIn = 3600
			}

			token := &Token{
				AccessToken:  tokens.AccessToken,
				RefreshToken: tokens.RefreshToken,
				AccountID:    accountID,
				ExpiresAt:    time.Now().Unix() + int64(expiresIn),
			}

			return token, nil
		}

		// 403/404 = still pending
		if status == 403 || status == 404 {
			continue
		}

		return nil, fmt.Errorf("unexpected poll response: %d", status)
	}

	return nil, fmt.Errorf("authorization timed out")
}

func pollDeviceToken(deviceAuthID, userCode string) (*deviceTokenResp, int, error) {
	body := fmt.Sprintf(`{"device_auth_id":"%s","user_code":"%s"}`, deviceAuthID, userCode)
	resp, err := postJSON(context.Background(), Issuer+"/api/accounts/deviceauth/token", body)
	if err != nil {
		return nil, 0, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return nil, resp.StatusCode, nil
	}

	var result deviceTokenResp
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, resp.StatusCode, err
	}
	return &result, 200, nil
}

func exchangeCode(ctx context.Context, authCode, codeVerifier string) (*oauthTokenResp, error) {
	data := url.Values{
		"grant_type":    {"authorization_code"},
		"code":          {authCode},
		"redirect_uri":  {Issuer + "/deviceauth/callback"},
		"client_id":     {ClientID},
		"code_verifier": {codeVerifier},
	}

	resp, err := postForm(ctx, Issuer+"/oauth/token", data)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		b, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("token exchange error (%d): %s", resp.StatusCode, string(b))
	}

	var tokens oauthTokenResp
	if err := json.NewDecoder(resp.Body).Decode(&tokens); err != nil {
		return nil, err
	}
	return &tokens, nil
}

// ── Token Refresh ────────────────────────────────────────────────────

// Refresh exchanges the previous token's refresh_token for a new access
// token. The OAuth2 spec allows the server to omit refresh_token in a
// refresh response (meaning "keep the existing one"); similarly the new
// id/access tokens may not contain the account id. In both cases we
// fall back to the value carried on `prev` so we don't brick the next
// refresh or drop the account id.
func Refresh(ctx context.Context, prev *Token) (*Token, error) {
	if prev == nil || prev.RefreshToken == "" {
		return nil, fmt.Errorf("refresh failed: no refresh token available")
	}

	data := url.Values{
		"grant_type":    {"refresh_token"},
		"refresh_token": {prev.RefreshToken},
		"client_id":     {ClientID},
	}

	resp, err := postForm(ctx, Issuer+"/oauth/token", data)
	if err != nil {
		return nil, fmt.Errorf("refresh failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		b, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("refresh error (%d): %s", resp.StatusCode, string(b))
	}

	var tokens oauthTokenResp
	if err := json.NewDecoder(resp.Body).Decode(&tokens); err != nil {
		return nil, err
	}

	accountID := extractAccountID(tokens.IDToken)
	if accountID == "" {
		accountID = extractAccountID(tokens.AccessToken)
	}
	if accountID == "" {
		// Provider didn't re-emit the account id — keep the old one.
		accountID = prev.AccountID
	}

	newRefresh := tokens.RefreshToken
	if newRefresh == "" {
		// Spec-compliant: server kept the existing refresh token.
		newRefresh = prev.RefreshToken
	}

	expiresIn := tokens.ExpiresIn
	if expiresIn == 0 {
		expiresIn = 3600
	}

	return &Token{
		AccessToken:  tokens.AccessToken,
		RefreshToken: newRefresh,
		AccountID:    accountID,
		ExpiresAt:    time.Now().Unix() + int64(expiresIn),
	}, nil
}

// ── JWT Account ID Extraction ────────────────────────────────────────

type jwtClaims struct {
	ChatGPTAccountID string `json:"chatgpt_account_id,omitempty"`
	OpenAIAuth       *struct {
		ChatGPTAccountID string `json:"chatgpt_account_id,omitempty"`
	} `json:"https://api.openai.com/auth,omitempty"`
	Organizations []struct {
		ID string `json:"id"`
	} `json:"organizations,omitempty"`
}

func extractAccountID(token string) string {
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		return ""
	}

	payload := parts[1]
	// Add padding
	switch len(payload) % 4 {
	case 2:
		payload += "=="
	case 3:
		payload += "="
	}

	decoded, err := base64.URLEncoding.DecodeString(payload)
	if err != nil {
		// Try without padding
		decoded, err = base64.RawURLEncoding.DecodeString(parts[1])
		if err != nil {
			return ""
		}
	}

	var claims jwtClaims
	if err := json.Unmarshal(decoded, &claims); err != nil {
		return ""
	}

	if claims.ChatGPTAccountID != "" {
		return claims.ChatGPTAccountID
	}
	if claims.OpenAIAuth != nil && claims.OpenAIAuth.ChatGPTAccountID != "" {
		return claims.OpenAIAuth.ChatGPTAccountID
	}
	if len(claims.Organizations) > 0 {
		return claims.Organizations[0].ID
	}
	return ""
}

// Token persistence (tokenPath, LoadToken, SaveToken, Logout) is now
// in accounts.go. The functions are kept with the same signatures for
// backward compatibility.
