package auth

import (
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strings"
)

// Identity holds the credentials needed to authenticate with Overleaf.
type Identity struct {
	Cookies   string
	CSRFToken string
	UserID    string
	Email     string
}

var (
	csrfInputRe = regexp.MustCompile(`<input[^>]*name="_csrf"[^>]*value="([^"]*)"`)
	csrfMetaRe  = regexp.MustCompile(`<meta\s+name="ol-csrfToken"\s+content="([^"]*)"`)
	userIDRe    = regexp.MustCompile(`<meta\s+name="ol-user_id"\s+content="([^"]*)"`)
	emailRe     = regexp.MustCompile(`<meta\s+name="ol-usersEmail"\s+content="([^"]*)"`)
)

// LoginWithCookies validates cookies by fetching the project dashboard page
// and extracting the CSRF token and user info from the HTML.
func LoginWithCookies(siteURL, cookies string) (*Identity, error) {
	siteURL = strings.TrimRight(siteURL, "/")

	// Extract just the cookie value (name=value part, before Domain/Path/etc)
	cookieVal := extractCookieValue(cookies)

	id := &Identity{Cookies: cookieVal}

	// Fetch the project page to validate cookies and extract metadata
	req, err := http.NewRequest("GET", siteURL+"/project", nil)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Cookie", cookieVal)
	req.Header.Set("Connection", "keep-alive")
	req.Header.Set("User-Agent", "downleaf/0.1")

	client := &http.Client{
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			// Don't follow redirects — a redirect to /login means invalid cookies
			return http.ErrUseLastResponse
		},
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request /project: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == 302 || resp.StatusCode == 301 {
		loc := resp.Header.Get("Location")
		return nil, fmt.Errorf("cookies invalid or expired (redirected to %s)", loc)
	}
	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("unexpected status %d from /project", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read response: %w", err)
	}
	html := string(body)

	// Extract CSRF token
	if m := csrfMetaRe.FindStringSubmatch(html); len(m) > 1 {
		id.CSRFToken = m[1]
	} else if m := csrfInputRe.FindStringSubmatch(html); len(m) > 1 {
		id.CSRFToken = m[1]
	}

	// Extract user ID and email
	if m := userIDRe.FindStringSubmatch(html); len(m) > 1 {
		id.UserID = m[1]
	}
	if m := emailRe.FindStringSubmatch(html); len(m) > 1 {
		id.Email = m[1]
	}

	if id.CSRFToken == "" {
		return nil, fmt.Errorf("could not extract CSRF token from /project page")
	}

	return id, nil
}

// extractCookieValue parses a raw cookie string (possibly with Domain, Path, etc)
// and returns only the name=value portion.
func extractCookieValue(raw string) string {
	// If the cookie string contains "; Domain=" or "; Path=", trim the extras
	parts := strings.Split(raw, ";")
	var cookieParts []string
	for _, p := range parts {
		p = strings.TrimSpace(p)
		lp := strings.ToLower(p)
		// Skip cookie attributes
		if strings.HasPrefix(lp, "domain=") ||
			strings.HasPrefix(lp, "path=") ||
			strings.HasPrefix(lp, "expires=") ||
			strings.HasPrefix(lp, "max-age=") ||
			lp == "httponly" || lp == "secure" ||
			strings.HasPrefix(lp, "samesite=") {
			continue
		}
		if p != "" {
			cookieParts = append(cookieParts, p)
		}
	}
	return strings.Join(cookieParts, "; ")
}
