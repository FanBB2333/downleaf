# Browser Login & Credential Management

## Overview

Add a native browser login window and multi-user credential management to Downleaf, eliminating the need for users to manually copy cookies from browser DevTools.

## 1. Browser Login Window (macOS Native WKWebView)

### Implementation

**`internal/auth/browser_darwin.go`** — CGo + Objective-C:

- Create an NSWindow (800x600, centered, titled "Login to Overleaf") containing a WKWebView
- Navigate to `siteURL + "/login"`
- WKNavigationDelegate monitors URL changes via `webView:didFinishNavigation:`
- When the URL path contains `/project`, login is successful
- Extract all cookies for the site's domain via `WKHTTPCookieStore.getAllCookies:`
  - Include HttpOnly cookies (not accessible via JavaScript)
  - Format as `name=value; name2=value2` string
  - Filter to only relevant cookies (session cookies for the Overleaf domain)
- Go side blocks on a channel waiting for the result
- Window close or timeout (5 minutes) returns an error
- Window runs on the main thread (required by AppKit)

**`internal/auth/browser_other.go`** — non-macOS fallback:

- Returns `ErrBrowserLoginNotSupported`
- Frontend falls back to manual cookie input

### Flow

1. User clicks "Login with Browser"
2. Native WKWebView window opens with Overleaf login page
3. User logs in via any method (Google OAuth, email/password, ORCID, etc.)
4. Overleaf redirects to `/project` on success
5. WKNavigationDelegate detects URL change, extracts cookies
6. Window closes automatically
7. Cookies returned to Go, passed to `auth.LoginWithCookies()` for validation
8. Credential auto-saved to store

## 2. Credential Storage

### Directory Structure

```
~/.downleaf/
  credentials/
    {id}.json          # Individual credential files (permission 0600)
```

### Credential Format

```json
{
  "id": "sha256(siteURL+email)[:12]",
  "siteURL": "https://www.overleaf.com",
  "email": "user@example.com",
  "userID": "65a1b2c3...",
  "cookies": "overleaf_session2=s%3A...",
  "csrfToken": "...",
  "createdAt": "2026-04-02T10:00:00Z",
  "lastUsedAt": "2026-04-02T10:00:00Z"
}
```

### `internal/credential/store.go`

- `type Store struct` — manages `~/.downleaf/credentials/` directory
- `NewStore() *Store` — creates directory if not exists
- `List() ([]CredentialInfo, error)` — returns summaries (id, email, siteURL, lastUsedAt), no sensitive fields
- `Save(cred *Credential) error` — write/update credential file with permission 0600
- `Load(id string) (*Credential, error)` — read full credential
- `Delete(id string) error` — remove credential file
- ID derived from `sha256(siteURL + email)[:12]` — same site+email overwrites

### Security

- File permissions 0600 (owner read/write only)
- No additional encryption (consistent with browser cookie storage)

## 3. Backend API

### New methods on `gui.App`

| Method | Signature | Description |
|--------|-----------|-------------|
| `LoginWithBrowser` | `(siteURL string) (*LoginStatus, error)` | Opens WKWebView login window, captures cookies, validates, saves credential |
| `ListCredentials` | `() ([]CredentialInfo, error)` | Lists saved credentials (summary only) |
| `LoginWithCredential` | `(id string) (*LoginStatus, error)` | Loads saved credential, validates cookies still work, logs in |
| `DeleteCredential` | `(id string) error` | Deletes a saved credential |
| `IsBrowserLoginSupported` | `() bool` | Returns true on macOS, false otherwise |

### LoginWithBrowser Flow

1. Call `auth.BrowserLogin(siteURL)` to open native window and get cookies
2. Call `auth.LoginWithCookies(siteURL, cookies)` to validate and extract identity
3. Save credential via `credential.Store.Save()`
4. Set `app.identity`, `app.client`, `app.siteURL`
5. Return `LoginStatus{LoggedIn: true, Email: ...}`

### LoginWithCredential Flow

1. Load credential from store
2. Call `auth.LoginWithCookies(cred.SiteURL, cred.Cookies)` to re-validate
3. If cookies expired, return error (frontend shows "Session expired, please login again")
4. Update `lastUsedAt`, save back
5. Set app state and return LoginStatus

## 4. Frontend Changes

### LoginPage Redesign

Layout (top to bottom):

1. **Site URL input** — shared between all login methods, defaults to `https://www.overleaf.com`
2. **Saved Accounts section** — list of credential cards:
   - Each card: email, site URL, last used time
   - Click card to login with that credential
   - Delete button (with confirmation) on each card
   - Hidden if no saved credentials
3. **"Login with Browser" button** — primary action, only shown when `IsBrowserLoginSupported()` is true
4. **Collapsible "Manual Cookie Login"** — existing textarea input, collapsed by default, expandable

### MainPage Changes

- User dropdown menu in top-right: add "Switch Account" item
- Clicking "Switch Account" calls `logout()` to return to LoginPage
- Saved credentials persist across sessions so the user sees their accounts on the LoginPage

## 5. Scope & Constraints

- macOS only for browser login (WKWebView); other platforms fall back to manual input
- Official overleaf.com supported first; self-hosted instances work via the site URL field
- No cookie encryption at rest (file permissions only)
- Credential validation on load: if cookies expired, prompt re-login
- Codex (GPT-5.4 xhigh) handles testing and bug fixes
