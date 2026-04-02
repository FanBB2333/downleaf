//go:build !darwin
// +build !darwin

package auth

import "errors"

// ErrBrowserLoginNotSupported is returned on platforms that don't support browser login.
var ErrBrowserLoginNotSupported = errors.New("browser login is not supported on this platform")

// ErrLoginCancelled is returned when the user closes the login window.
var ErrLoginCancelled = errors.New("login cancelled by user")

// ErrLoginTimeout is returned when login times out.
var ErrLoginTimeout = errors.New("login timed out")

// BrowserLogin is not supported on non-macOS platforms.
func BrowserLogin(siteURL string) (string, error) {
	return "", ErrBrowserLoginNotSupported
}

// BrowserLoginWithOptions is not supported on non-macOS platforms.
func BrowserLoginWithOptions(siteURL, loginPath, successPath string, timeoutSeconds int) (string, error) {
	return "", ErrBrowserLoginNotSupported
}

// IsBrowserLoginSupported returns false on non-macOS platforms.
func IsBrowserLoginSupported() bool {
	return false
}
