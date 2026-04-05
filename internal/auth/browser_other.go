//go:build !darwin
// +build !darwin

package auth

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"os/exec"
	"runtime"
	"strings"
	"time"
)

// ErrBrowserLoginNotSupported is returned on platforms that don't support browser login.
var ErrBrowserLoginNotSupported = errors.New("browser login is not supported on this platform")

// ErrLoginCancelled is returned when the user closes the login window.
var ErrLoginCancelled = errors.New("login cancelled by user")

// ErrLoginTimeout is returned when login times out.
var ErrLoginTimeout = errors.New("login timed out")

// BrowserLogin opens the system browser for Overleaf login and captures
// cookies via a local callback server. The user logs in normally, then
// clicks a helper link to send cookies back.
func BrowserLogin(siteURL string) (string, error) {
	return BrowserLoginWithOptions(siteURL, "/login", "/project", 300)
}

// BrowserLoginWithOptions allows customizing paths and timeout.
func BrowserLoginWithOptions(siteURL, loginPath, successPath string, timeoutSeconds int) (string, error) {
	siteURL = strings.TrimRight(siteURL, "/")

	// Find a free port for the callback server
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return "", fmt.Errorf("could not start callback server: %w", err)
	}
	port := listener.Addr().(*net.TCPAddr).Port
	callbackURL := fmt.Sprintf("http://127.0.0.1:%d", port)

	cookieCh := make(chan string, 1)
	errCh := make(chan error, 1)

	mux := http.NewServeMux()

	// Callback endpoint: receives cookies from the helper page
	mux.HandleFunc("/callback", func(w http.ResponseWriter, r *http.Request) {
		cookies := r.URL.Query().Get("cookies")
		if cookies == "" {
			http.Error(w, "No cookies received", http.StatusBadRequest)
			return
		}
		w.Header().Set("Content-Type", "text/html")
		fmt.Fprint(w, `<html><body><h2>Login successful!</h2><p>You can close this tab and return to the terminal.</p></body></html>`)
		cookieCh <- cookies
	})

	// Helper page: JavaScript extracts cookies and sends to callback
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		fmt.Fprintf(w, `<!DOCTYPE html>
<html>
<head><title>Downleaf Login Helper</title></head>
<body>
<h2>Downleaf Login Helper</h2>
<p>After logging in to Overleaf, click the button below to complete authentication:</p>
<ol>
  <li>Log in at <a href="%s%s" target="_blank">%s%s</a></li>
  <li>After successful login (you see your projects), come back here</li>
  <li>Click the button below</li>
</ol>
<p><strong>Or</strong> paste your cookie manually:</p>
<textarea id="cookie-input" rows="3" cols="60" placeholder="overleaf_session2=..."></textarea>
<br><br>
<button onclick="submitCookie()">Submit Cookie</button>
<script>
function submitCookie() {
  var cookie = document.getElementById('cookie-input').value.trim();
  if (!cookie) {
    alert('Please paste your cookie first');
    return;
  }
  window.location = '/callback?cookies=' + encodeURIComponent(cookie);
}
</script>
</body>
</html>`, siteURL, loginPath, siteURL, loginPath)
	})

	srv := &http.Server{Handler: mux}
	go func() {
		if err := srv.Serve(listener); err != nil && err != http.ErrServerClosed {
			errCh <- err
		}
	}()

	// Open browser
	fmt.Printf("Opening browser for login...\n")
	fmt.Printf("If the browser doesn't open, visit: %s\n", callbackURL)
	openBrowser(callbackURL)

	// Wait for cookies or timeout
	select {
	case cookies := <-cookieCh:
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		srv.Shutdown(ctx)
		return cookies, nil
	case err := <-errCh:
		return "", err
	case <-time.After(time.Duration(timeoutSeconds) * time.Second):
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		srv.Shutdown(ctx)
		return "", ErrLoginTimeout
	}
}

// IsBrowserLoginSupported returns true — all platforms support the
// browser-based flow (via system browser + local callback server).
func IsBrowserLoginSupported() bool {
	return true
}

func openBrowser(url string) {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "linux":
		cmd = exec.Command("xdg-open", url)
	case "windows":
		cmd = exec.Command("cmd", "/c", "start", url)
	default:
		cmd = exec.Command("open", url)
	}
	cmd.Start()
}
