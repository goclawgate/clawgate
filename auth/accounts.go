package auth

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

// accountNameRe defines the valid account name pattern.
var accountNameRe = regexp.MustCompile(`^[a-z0-9][a-z0-9._-]{0,31}$`)

// reservedNames lists account names that cannot be used.
var reservedNames = map[string]bool{
	"all": true,
}

// StoredAccount is a single persisted account entry.
type StoredAccount struct {
	Name         string `json:"name"`
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	AccountID    string `json:"account_id,omitempty"`
	ExpiresAt    int64  `json:"expires_at"`
}

// Token converts a StoredAccount to the existing Token type used by
// the OAuth and refresh logic.
func (a *StoredAccount) Token() *Token {
	return &Token{
		AccessToken:  a.AccessToken,
		RefreshToken: a.RefreshToken,
		AccountID:    a.AccountID,
		ExpiresAt:    a.ExpiresAt,
	}
}

// FromToken populates the account fields from a Token, preserving Name.
func (a *StoredAccount) FromToken(t *Token) {
	a.AccessToken = t.AccessToken
	a.RefreshToken = t.RefreshToken
	a.AccountID = t.AccountID
	a.ExpiresAt = t.ExpiresAt
}

// Store is the v2 multi-account storage format.
type Store struct {
	Version        int              `json:"version"`
	DefaultAccount string           `json:"default_account"`
	Accounts       []StoredAccount  `json:"accounts"`
}

// ValidateAccountName checks that name is a valid account name.
func ValidateAccountName(name string) error {
	if reservedNames[name] {
		return fmt.Errorf("account name %q is reserved", name)
	}
	if !accountNameRe.MatchString(name) {
		return fmt.Errorf("account name %q is invalid: must match %s", name, accountNameRe.String())
	}
	return nil
}

// tokenPath returns the path to the token store file, creating the
// directory if needed.
func tokenPath() string {
	home, _ := os.UserHomeDir()
	dir := filepath.Join(home, ".clawgate")
	os.MkdirAll(dir, 0700)
	return filepath.Join(dir, "token.json")
}

// TokenPath is the exported accessor for the token store path, used
// by main.go for full-file removal during logout.
func TokenPath() string { return tokenPath() }

// LoadStore reads the account store from disk. If the file contains a
// legacy v1 single-token payload, it is transparently migrated to v2
// format in memory (not written back until a mutating save).
func LoadStore() (*Store, error) {
	data, err := os.ReadFile(tokenPath())
	if err != nil {
		return nil, err
	}

	// Try v2 first.
	var s Store
	if err := json.Unmarshal(data, &s); err == nil && s.Version >= 2 {
		return &s, nil
	}

	// Fall back to legacy single-token format.
	var t Token
	if err := json.Unmarshal(data, &t); err != nil {
		return nil, fmt.Errorf("failed to parse token store: %w", err)
	}
	if t.AccessToken == "" {
		return nil, fmt.Errorf("empty token in legacy format")
	}

	return &Store{
		Version:        2,
		DefaultAccount: "default",
		Accounts: []StoredAccount{
			{
				Name:         "default",
				AccessToken:  t.AccessToken,
				RefreshToken: t.RefreshToken,
				AccountID:    t.AccountID,
				ExpiresAt:    t.ExpiresAt,
			},
		},
	}, nil
}

// SaveStore writes the store to disk atomically via temp-file + rename.
func SaveStore(s *Store) error {
	s.Version = 2
	data, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return err
	}

	path := tokenPath()
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0600); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

// ResolveAccount returns the account for the given name. If name is
// empty, it returns the default account. When there is exactly one
// account and no explicit default, that account is auto-resolved.
func (s *Store) ResolveAccount(name string) (*StoredAccount, error) {
	if len(s.Accounts) == 0 {
		return nil, fmt.Errorf("no accounts saved — run 'clawgate login' first")
	}

	if name == "" {
		name = s.DefaultAccount
	}

	// If still empty (no default set) and exactly one account, use it.
	if name == "" {
		if len(s.Accounts) == 1 {
			return &s.Accounts[0], nil
		}
		return nil, fmt.Errorf("multiple accounts saved but no default set — run 'clawgate account use NAME'")
	}

	for i := range s.Accounts {
		if s.Accounts[i].Name == name {
			return &s.Accounts[i], nil
		}
	}

	return nil, fmt.Errorf("account %q not found (saved accounts: %s)", name, s.accountList())
}

// UpsertAccount adds or replaces an account. If replace is false and
// the account already exists, an error is returned. The first account
// ever added auto-becomes the default.
func (s *Store) UpsertAccount(acct StoredAccount, replace bool) error {
	for i, a := range s.Accounts {
		if a.Name == acct.Name {
			if !replace {
				return fmt.Errorf("account %q already exists — use --replace to overwrite", acct.Name)
			}
			s.Accounts[i] = acct
			return nil
		}
	}
	s.Accounts = append(s.Accounts, acct)
	if len(s.Accounts) == 1 {
		s.DefaultAccount = acct.Name
	}
	return nil
}

// RemoveAccount removes the named account. If the removed account was
// the default, DefaultAccount is cleared.
func (s *Store) RemoveAccount(name string) error {
	for i, a := range s.Accounts {
		if a.Name == name {
			s.Accounts = append(s.Accounts[:i], s.Accounts[i+1:]...)
			if s.DefaultAccount == name {
				s.DefaultAccount = ""
			}
			return nil
		}
	}
	return fmt.Errorf("account %q not found", name)
}

// RemoveAll removes every account and clears the default.
func (s *Store) RemoveAll() {
	s.Accounts = nil
	s.DefaultAccount = ""
}

// SetDefault changes the persistent default account.
func (s *Store) SetDefault(name string) error {
	for _, a := range s.Accounts {
		if a.Name == name {
			s.DefaultAccount = name
			return nil
		}
	}
	return fmt.Errorf("account %q not found (saved accounts: %s)", name, s.accountList())
}

// AccountNames returns sorted account names.
func (s *Store) AccountNames() []string {
	names := make([]string, len(s.Accounts))
	for i, a := range s.Accounts {
		names[i] = a.Name
	}
	sort.Strings(names)
	return names
}

// GetDefault returns the default account name.
func (s *Store) GetDefault() string {
	return s.DefaultAccount
}

// accountList returns a comma-separated list of account names for
// error messages.
func (s *Store) accountList() string {
	return strings.Join(s.AccountNames(), ", ")
}

// ── Compatibility shims ─────────────────────────────────────────────
// These keep existing callers (tests, handler) working until they are
// updated to use the Store API directly.

// LoadToken loads the default account's token from the store.
func LoadToken() (*Token, error) {
	s, err := LoadStore()
	if err != nil {
		return nil, err
	}
	acct, err := s.ResolveAccount("")
	if err != nil {
		return nil, err
	}
	return acct.Token(), nil
}

// SaveToken saves a token as the "default" account in the store.
func SaveToken(t *Token) error {
	s, _ := LoadStore()
	if s == nil {
		s = &Store{Version: 2}
	}
	acct := StoredAccount{Name: "default"}
	acct.FromToken(t)
	// Use replace=true so re-saving always works.
	if err := s.UpsertAccount(acct, true); err != nil {
		return err
	}
	if s.DefaultAccount == "" {
		s.DefaultAccount = "default"
	}
	return SaveStore(s)
}

// Logout removes the token file entirely.
func Logout() {
	os.Remove(tokenPath())
	fmt.Println("Logged out — token removed")
}
