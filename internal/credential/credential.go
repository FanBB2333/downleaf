// Package credential provides secure storage for Overleaf login credentials.
package credential

import (
	"crypto/sha256"
	"encoding/hex"
	"time"
)

// Credential holds the full authentication data for an Overleaf account.
type Credential struct {
	ID         string    `json:"id"`
	SiteURL    string    `json:"siteURL"`
	Email      string    `json:"email"`
	UserID     string    `json:"userID"`
	Cookies    string    `json:"cookies"`
	CSRFToken  string    `json:"csrfToken"`
	CreatedAt  time.Time `json:"createdAt"`
	LastUsedAt time.Time `json:"lastUsedAt"`
}

// CredentialInfo is a summary of a credential without sensitive fields.
// Used for listing saved accounts in the UI.
type CredentialInfo struct {
	ID         string    `json:"id"`
	SiteURL    string    `json:"siteURL"`
	Email      string    `json:"email"`
	LastUsedAt time.Time `json:"lastUsedAt"`
}

// Info returns a CredentialInfo summary of the credential.
func (c *Credential) Info() CredentialInfo {
	return CredentialInfo{
		ID:         c.ID,
		SiteURL:    c.SiteURL,
		Email:      c.Email,
		LastUsedAt: c.LastUsedAt,
	}
}

// GenerateID creates a deterministic ID from siteURL and email.
// Same site+email will always produce the same ID, allowing overwrites.
func GenerateID(siteURL, email string) string {
	h := sha256.Sum256([]byte(siteURL + email))
	return hex.EncodeToString(h[:])[:12]
}
