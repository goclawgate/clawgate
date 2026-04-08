package auth

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// setupTestHome points the token store at a temp directory.
func setupTestHome(t *testing.T) string {
	t.Helper()
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)
	t.Setenv("USERPROFILE", tmp) // Windows
	return tmp
}

// TestLegacyV1Migration verifies that a legacy single-token file is
// transparently loaded as a v2 store with one account named "default".
func TestLegacyV1Migration(t *testing.T) {
	tmp := setupTestHome(t)
	dir := filepath.Join(tmp, ".clawgate")
	os.MkdirAll(dir, 0700)

	legacy := Token{
		AccessToken:  "old-access",
		RefreshToken: "old-refresh",
		AccountID:    "acct_1",
		ExpiresAt:    1700000000,
	}
	data, _ := json.Marshal(legacy)
	os.WriteFile(filepath.Join(dir, "token.json"), data, 0600)

	s, err := LoadStore()
	if err != nil {
		t.Fatalf("LoadStore failed: %v", err)
	}
	if s.Version != 2 {
		t.Errorf("Version = %d, want 2", s.Version)
	}
	if s.DefaultAccount != "default" {
		t.Errorf("DefaultAccount = %q, want %q", s.DefaultAccount, "default")
	}
	if len(s.Accounts) != 1 {
		t.Fatalf("expected 1 account, got %d", len(s.Accounts))
	}
	a := s.Accounts[0]
	if a.Name != "default" || a.AccessToken != "old-access" || a.RefreshToken != "old-refresh" {
		t.Errorf("unexpected account: %+v", a)
	}
}

// TestV2RoundTrip verifies save + load preserves all fields.
func TestV2RoundTrip(t *testing.T) {
	setupTestHome(t)

	s := &Store{
		Version:        2,
		DefaultAccount: "work",
		Accounts: []StoredAccount{
			{Name: "default", AccessToken: "a1", RefreshToken: "r1", AccountID: "id1", ExpiresAt: 100},
			{Name: "work", AccessToken: "a2", RefreshToken: "r2", AccountID: "id2", ExpiresAt: 200},
		},
	}
	if err := SaveStore(s); err != nil {
		t.Fatalf("SaveStore: %v", err)
	}

	loaded, err := LoadStore()
	if err != nil {
		t.Fatalf("LoadStore: %v", err)
	}
	if loaded.DefaultAccount != "work" {
		t.Errorf("DefaultAccount = %q, want %q", loaded.DefaultAccount, "work")
	}
	if len(loaded.Accounts) != 2 {
		t.Fatalf("expected 2 accounts, got %d", len(loaded.Accounts))
	}
	if loaded.Accounts[1].AccessToken != "a2" {
		t.Errorf("second account AccessToken = %q, want %q", loaded.Accounts[1].AccessToken, "a2")
	}
}

// TestValidateAccountName checks valid names, too-long, uppercase,
// reserved "all", and special characters.
func TestValidateAccountName(t *testing.T) {
	tests := []struct {
		name    string
		wantErr bool
	}{
		{"default", false},
		{"work", false},
		{"my-org.team_1", false},
		{"a", false},
		{"0start", false},
		// Invalid cases
		{"All", true},   // uppercase (doesn't match regex)
		{"all", true},   // reserved
		{"", true},      // empty
		{"-start", true}, // starts with hyphen
		{".start", true}, // starts with dot
		{"has space", true},
		{"UPPER", true},
		{strings.Repeat("a", 33), true}, // too long
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateAccountName(tt.name)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateAccountName(%q) error = %v, wantErr %v", tt.name, err, tt.wantErr)
			}
		})
	}
}

// TestUpsertAccountNew adds a new account successfully.
func TestUpsertAccountNew(t *testing.T) {
	s := &Store{Version: 2}
	acct := StoredAccount{Name: "work", AccessToken: "tok"}
	if err := s.UpsertAccount(acct, false); err != nil {
		t.Fatalf("UpsertAccount: %v", err)
	}
	if len(s.Accounts) != 1 {
		t.Fatalf("expected 1 account, got %d", len(s.Accounts))
	}
}

// TestUpsertAccountDuplicateNoReplace fails when account exists.
func TestUpsertAccountDuplicateNoReplace(t *testing.T) {
	s := &Store{Version: 2, Accounts: []StoredAccount{{Name: "work"}}}
	err := s.UpsertAccount(StoredAccount{Name: "work"}, false)
	if err == nil {
		t.Fatal("expected error for duplicate without replace")
	}
}

// TestUpsertAccountDuplicateWithReplace succeeds and overwrites.
func TestUpsertAccountDuplicateWithReplace(t *testing.T) {
	s := &Store{Version: 2, Accounts: []StoredAccount{{Name: "work", AccessToken: "old"}}}
	err := s.UpsertAccount(StoredAccount{Name: "work", AccessToken: "new"}, true)
	if err != nil {
		t.Fatalf("UpsertAccount with replace: %v", err)
	}
	if s.Accounts[0].AccessToken != "new" {
		t.Errorf("AccessToken = %q, want %q", s.Accounts[0].AccessToken, "new")
	}
}

// TestFirstAccountAutoDefault verifies the first account becomes default.
func TestFirstAccountAutoDefault(t *testing.T) {
	s := &Store{Version: 2}
	s.UpsertAccount(StoredAccount{Name: "first"}, false)
	if s.DefaultAccount != "first" {
		t.Errorf("DefaultAccount = %q, want %q", s.DefaultAccount, "first")
	}
	// Second account should NOT change default.
	s.UpsertAccount(StoredAccount{Name: "second"}, false)
	if s.DefaultAccount != "first" {
		t.Errorf("DefaultAccount changed to %q after adding second account", s.DefaultAccount)
	}
}

// TestRemoveAccountClearsDefault verifies removing the default account
// clears DefaultAccount.
func TestRemoveAccountClearsDefault(t *testing.T) {
	s := &Store{
		Version:        2,
		DefaultAccount: "work",
		Accounts:       []StoredAccount{{Name: "default"}, {Name: "work"}},
	}
	if err := s.RemoveAccount("work"); err != nil {
		t.Fatalf("RemoveAccount: %v", err)
	}
	if s.DefaultAccount != "" {
		t.Errorf("DefaultAccount should be empty after removing default, got %q", s.DefaultAccount)
	}
}

// TestRemoveAccountKeepsDefault verifies removing a non-default account
// preserves the default.
func TestRemoveAccountKeepsDefault(t *testing.T) {
	s := &Store{
		Version:        2,
		DefaultAccount: "work",
		Accounts:       []StoredAccount{{Name: "default"}, {Name: "work"}},
	}
	if err := s.RemoveAccount("default"); err != nil {
		t.Fatalf("RemoveAccount: %v", err)
	}
	if s.DefaultAccount != "work" {
		t.Errorf("DefaultAccount = %q, want %q", s.DefaultAccount, "work")
	}
}

// TestRemoveAccountNotFound returns an error for missing accounts.
func TestRemoveAccountNotFound(t *testing.T) {
	s := &Store{Version: 2}
	if err := s.RemoveAccount("nope"); err == nil {
		t.Fatal("expected error for missing account")
	}
}

// TestResolveAccountEmptyOneAccount auto-resolves the single account.
func TestResolveAccountEmptyOneAccount(t *testing.T) {
	s := &Store{
		Version:  2,
		Accounts: []StoredAccount{{Name: "solo", AccessToken: "tok"}},
	}
	acct, err := s.ResolveAccount("")
	if err != nil {
		t.Fatalf("ResolveAccount: %v", err)
	}
	if acct.Name != "solo" {
		t.Errorf("resolved %q, want %q", acct.Name, "solo")
	}
}

// TestResolveAccountEmptyMultipleNoDefault returns an error.
func TestResolveAccountEmptyMultipleNoDefault(t *testing.T) {
	s := &Store{
		Version:  2,
		Accounts: []StoredAccount{{Name: "a"}, {Name: "b"}},
	}
	_, err := s.ResolveAccount("")
	if err == nil {
		t.Fatal("expected error when multiple accounts and no default")
	}
}

// TestResolveAccountByName finds the named account.
func TestResolveAccountByName(t *testing.T) {
	s := &Store{
		Version:        2,
		DefaultAccount: "a",
		Accounts:       []StoredAccount{{Name: "a"}, {Name: "b", AccessToken: "btok"}},
	}
	acct, err := s.ResolveAccount("b")
	if err != nil {
		t.Fatalf("ResolveAccount: %v", err)
	}
	if acct.AccessToken != "btok" {
		t.Errorf("AccessToken = %q, want %q", acct.AccessToken, "btok")
	}
}

// TestResolveAccountNotFound returns an error listing saved accounts.
func TestResolveAccountNotFound(t *testing.T) {
	s := &Store{
		Version:  2,
		Accounts: []StoredAccount{{Name: "a"}, {Name: "b"}},
	}
	_, err := s.ResolveAccount("missing")
	if err == nil {
		t.Fatal("expected error for missing account")
	}
	if !strings.Contains(err.Error(), "a, b") {
		t.Errorf("error should list saved accounts, got: %v", err)
	}
}

// TestSetDefault changes the default and errors on missing name.
func TestSetDefault(t *testing.T) {
	s := &Store{
		Version:        2,
		DefaultAccount: "a",
		Accounts:       []StoredAccount{{Name: "a"}, {Name: "b"}},
	}
	if err := s.SetDefault("b"); err != nil {
		t.Fatalf("SetDefault: %v", err)
	}
	if s.DefaultAccount != "b" {
		t.Errorf("DefaultAccount = %q, want %q", s.DefaultAccount, "b")
	}
	if err := s.SetDefault("nope"); err == nil {
		t.Fatal("expected error for missing account")
	}
}

// TestRemoveAll removes everything.
func TestRemoveAll(t *testing.T) {
	s := &Store{
		Version:        2,
		DefaultAccount: "x",
		Accounts:       []StoredAccount{{Name: "x"}, {Name: "y"}},
	}
	s.RemoveAll()
	if len(s.Accounts) != 0 || s.DefaultAccount != "" {
		t.Errorf("RemoveAll should clear accounts and default")
	}
}

// TestAccountNames returns sorted names.
func TestAccountNames(t *testing.T) {
	s := &Store{Accounts: []StoredAccount{{Name: "z"}, {Name: "a"}, {Name: "m"}}}
	names := s.AccountNames()
	if len(names) != 3 || names[0] != "a" || names[1] != "m" || names[2] != "z" {
		t.Errorf("AccountNames = %v, want [a m z]", names)
	}
}

// TestAtomicWrite verifies the temp-file pattern doesn't leave
// partial files on disk.
func TestAtomicWrite(t *testing.T) {
	setupTestHome(t)

	s := &Store{Version: 2, Accounts: []StoredAccount{{Name: "test", AccessToken: "tok"}}}
	if err := SaveStore(s); err != nil {
		t.Fatalf("SaveStore: %v", err)
	}

	// Verify no .tmp file left behind.
	path := tokenPath()
	if _, err := os.Stat(path + ".tmp"); !os.IsNotExist(err) {
		t.Error("temp file should not exist after successful save")
	}

	// Verify the written file is valid JSON.
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	var loaded Store
	if err := json.Unmarshal(data, &loaded); err != nil {
		t.Fatalf("written file is not valid JSON: %v", err)
	}
	if loaded.Version != 2 {
		t.Errorf("Version = %d, want 2", loaded.Version)
	}
}

// TestStoredAccountTokenConversion verifies Token() and FromToken().
func TestStoredAccountTokenConversion(t *testing.T) {
	acct := StoredAccount{Name: "x", AccessToken: "a", RefreshToken: "r", AccountID: "id", ExpiresAt: 999}
	tok := acct.Token()
	if tok.AccessToken != "a" || tok.RefreshToken != "r" || tok.AccountID != "id" || tok.ExpiresAt != 999 {
		t.Errorf("Token() mismatch: %+v", tok)
	}

	newTok := &Token{AccessToken: "a2", RefreshToken: "r2", AccountID: "id2", ExpiresAt: 111}
	acct.FromToken(newTok)
	if acct.Name != "x" {
		t.Error("FromToken should preserve Name")
	}
	if acct.AccessToken != "a2" {
		t.Errorf("FromToken AccessToken = %q, want %q", acct.AccessToken, "a2")
	}
}

// TestCompatShimLoadToken verifies the LoadToken shim works with v2 store.
func TestCompatShimLoadToken(t *testing.T) {
	setupTestHome(t)

	s := &Store{
		Version:        2,
		DefaultAccount: "default",
		Accounts:       []StoredAccount{{Name: "default", AccessToken: "tok", RefreshToken: "ref", ExpiresAt: 9999999999}},
	}
	if err := SaveStore(s); err != nil {
		t.Fatalf("SaveStore: %v", err)
	}

	tok, err := LoadToken()
	if err != nil {
		t.Fatalf("LoadToken: %v", err)
	}
	if tok.AccessToken != "tok" {
		t.Errorf("AccessToken = %q, want %q", tok.AccessToken, "tok")
	}
}

// TestCompatShimSaveToken verifies the SaveToken shim writes v2 format.
func TestCompatShimSaveToken(t *testing.T) {
	setupTestHome(t)

	tok := &Token{AccessToken: "a", RefreshToken: "r", AccountID: "id", ExpiresAt: 100}
	if err := SaveToken(tok); err != nil {
		t.Fatalf("SaveToken: %v", err)
	}

	s, err := LoadStore()
	if err != nil {
		t.Fatalf("LoadStore: %v", err)
	}
	if s.Version != 2 {
		t.Errorf("Version = %d, want 2", s.Version)
	}
	if len(s.Accounts) != 1 || s.Accounts[0].Name != "default" {
		t.Errorf("expected one 'default' account, got %+v", s.Accounts)
	}
}
