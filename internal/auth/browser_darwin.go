//go:build darwin
// +build darwin

package auth

/*
#cgo CFLAGS: -x objective-c
#cgo LDFLAGS: -framework Cocoa -framework WebKit

#import <Cocoa/Cocoa.h>
#import <WebKit/WebKit.h>

// Result structure passed back to Go
typedef struct {
    char* cookies;  // Semicolon-separated cookie string, or NULL on error
    char* error;    // Error message, or NULL on success
} BrowserLoginResult;

// Forward declaration
BrowserLoginResult runBrowserLogin(const char* siteURL, const char* loginPath, const char* successPath, int timeoutSeconds);

// Delegate to monitor navigation and extract cookies
@interface LoginDelegate : NSObject <WKNavigationDelegate>
@property (nonatomic, strong) NSString* siteURL;
@property (nonatomic, strong) NSString* successPath;
@property (nonatomic, strong) NSString* cookies;
@property (nonatomic, strong) NSString* errorMsg;
@property (nonatomic, assign) BOOL completed;
@property (nonatomic, strong) NSWindow* window;
@property (nonatomic, strong) WKWebView* webView;
@end

@implementation LoginDelegate

- (void)webView:(WKWebView *)webView didFinishNavigation:(WKNavigation *)navigation {
    NSURL* url = webView.URL;
    if (!url) return;

    NSString* path = url.path;
    // Check if we've reached the success path (e.g., /project)
    if ([path hasPrefix:self.successPath]) {
        // Extract cookies for this domain
        WKHTTPCookieStore* cookieStore = webView.configuration.websiteDataStore.httpCookieStore;
        [cookieStore getAllCookies:^(NSArray<NSHTTPCookie*>* cookies) {
            NSMutableArray* parts = [NSMutableArray array];
            NSURL* siteURLObj = [NSURL URLWithString:self.siteURL];
            NSString* siteDomain = siteURLObj.host;

            for (NSHTTPCookie* cookie in cookies) {
                // Include cookies that match the site domain (including subdomains)
                NSString* cookieDomain = cookie.domain;
                if ([cookieDomain hasPrefix:@"."]) {
                    cookieDomain = [cookieDomain substringFromIndex:1];
                }
                if ([siteDomain hasSuffix:cookieDomain] || [siteDomain isEqualToString:cookieDomain]) {
                    [parts addObject:[NSString stringWithFormat:@"%@=%@", cookie.name, cookie.value]];
                }
            }

            self.cookies = [parts componentsJoinedByString:@"; "];
            self.completed = YES;

            // Close window on main thread
            dispatch_async(dispatch_get_main_queue(), ^{
                [self.window close];
                [NSApp stopModal];
            });
        }];
    }
}

- (void)webView:(WKWebView *)webView didFailNavigation:(WKNavigation *)navigation withError:(NSError *)error {
    self.errorMsg = [error localizedDescription];
    self.completed = YES;
    dispatch_async(dispatch_get_main_queue(), ^{
        [self.window close];
        [NSApp stopModal];
    });
}

- (void)webView:(WKWebView *)webView didFailProvisionalNavigation:(WKNavigation *)navigation withError:(NSError *)error {
    // Ignore cancellation errors (happen during redirects)
    if (error.code == NSURLErrorCancelled) return;

    self.errorMsg = [error localizedDescription];
    self.completed = YES;
    dispatch_async(dispatch_get_main_queue(), ^{
        [self.window close];
        [NSApp stopModal];
    });
}

@end

// Window delegate to handle close button
@interface WindowCloseDelegate : NSObject <NSWindowDelegate>
@property (nonatomic, unsafe_unretained) LoginDelegate* loginDelegate;
@end

@implementation WindowCloseDelegate
- (BOOL)windowShouldClose:(NSWindow *)sender {
    if (!self.loginDelegate.completed) {
        self.loginDelegate.errorMsg = @"Login cancelled by user";
        self.loginDelegate.completed = YES;
    }
    [NSApp stopModal];
    return YES;
}
@end

BrowserLoginResult runBrowserLogin(const char* siteURL, const char* loginPath, const char* successPath, int timeoutSeconds) {
    BrowserLoginResult result = {NULL, NULL};

    @autoreleasepool {
        // Ensure we have an NSApplication
        [NSApplication sharedApplication];
        [NSApp setActivationPolicy:NSApplicationActivationPolicyAccessory];

        NSString* siteURLStr = [NSString stringWithUTF8String:siteURL];
        NSString* loginPathStr = [NSString stringWithUTF8String:loginPath];
        NSString* successPathStr = [NSString stringWithUTF8String:successPath];

        // Create window
        NSRect frame = NSMakeRect(0, 0, 800, 600);
        NSWindow* window = [[NSWindow alloc] initWithContentRect:frame
                                                       styleMask:(NSWindowStyleMaskTitled |
                                                                 NSWindowStyleMaskClosable |
                                                                 NSWindowStyleMaskResizable)
                                                         backing:NSBackingStoreBuffered
                                                           defer:NO];
        [window setTitle:@"Login to Overleaf"];
        [window center];

        // Create WKWebView with persistent data store to handle cookies properly
        WKWebViewConfiguration* config = [[WKWebViewConfiguration alloc] init];
        config.websiteDataStore = [WKWebsiteDataStore defaultDataStore];

        WKWebView* webView = [[WKWebView alloc] initWithFrame:frame configuration:config];
        [window setContentView:webView];

        // Set up delegate
        LoginDelegate* delegate = [[LoginDelegate alloc] init];
        delegate.siteURL = siteURLStr;
        delegate.successPath = successPathStr;
        delegate.completed = NO;
        delegate.window = window;
        delegate.webView = webView;
        webView.navigationDelegate = delegate;

        // Window close delegate
        WindowCloseDelegate* windowDelegate = [[WindowCloseDelegate alloc] init];
        windowDelegate.loginDelegate = delegate;
        window.delegate = windowDelegate;

        // Load login URL
        NSString* fullURL = [siteURLStr stringByAppendingString:loginPathStr];
        NSURL* url = [NSURL URLWithString:fullURL];
        NSURLRequest* request = [NSURLRequest requestWithURL:url];
        [webView loadRequest:request];

        // Show window
        [window makeKeyAndOrderFront:nil];
        [NSApp activateIgnoringOtherApps:YES];

        // Run modal with timeout
        dispatch_after(dispatch_time(DISPATCH_TIME_NOW, (int64_t)(timeoutSeconds * NSEC_PER_SEC)),
                      dispatch_get_main_queue(), ^{
            if (!delegate.completed) {
                delegate.errorMsg = @"Login timed out";
                delegate.completed = YES;
                [window close];
                [NSApp stopModal];
            }
        });

        [NSApp runModalForWindow:window];

        // Collect result
        if (delegate.errorMsg) {
            result.error = strdup([delegate.errorMsg UTF8String]);
        } else if (delegate.cookies) {
            result.cookies = strdup([delegate.cookies UTF8String]);
        } else {
            result.error = strdup("Unknown error: no cookies captured");
        }
    }

    return result;
}
*/
import "C"

import (
	"errors"
	"runtime"
	"unsafe"
)

// ErrBrowserLoginNotSupported is returned on platforms that don't support browser login.
var ErrBrowserLoginNotSupported = errors.New("browser login is not supported on this platform")

// ErrLoginCancelled is returned when the user closes the login window.
var ErrLoginCancelled = errors.New("login cancelled by user")

// ErrLoginTimeout is returned when login times out.
var ErrLoginTimeout = errors.New("login timed out")

const (
	defaultLoginPath   = "/login"
	defaultSuccessPath = "/project"
	defaultTimeout     = 300 // 5 minutes
)

// BrowserLogin opens a native browser window for Overleaf login.
// On success, returns the captured cookies as a semicolon-separated string.
// This function blocks until login completes, times out, or is cancelled.
func BrowserLogin(siteURL string) (string, error) {
	return BrowserLoginWithOptions(siteURL, defaultLoginPath, defaultSuccessPath, defaultTimeout)
}

// BrowserLoginWithOptions allows customizing login/success paths and timeout.
func BrowserLoginWithOptions(siteURL, loginPath, successPath string, timeoutSeconds int) (string, error) {
	// Ensure we run on the main thread (required by AppKit)
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()

	cSiteURL := C.CString(siteURL)
	cLoginPath := C.CString(loginPath)
	cSuccessPath := C.CString(successPath)
	defer C.free(unsafe.Pointer(cSiteURL))
	defer C.free(unsafe.Pointer(cLoginPath))
	defer C.free(unsafe.Pointer(cSuccessPath))

	result := C.runBrowserLogin(cSiteURL, cLoginPath, cSuccessPath, C.int(timeoutSeconds))

	if result.error != nil {
		errMsg := C.GoString(result.error)
		C.free(unsafe.Pointer(result.error))

		switch errMsg {
		case "Login cancelled by user":
			return "", ErrLoginCancelled
		case "Login timed out":
			return "", ErrLoginTimeout
		default:
			return "", errors.New(errMsg)
		}
	}

	if result.cookies != nil {
		cookies := C.GoString(result.cookies)
		C.free(unsafe.Pointer(result.cookies))
		return cookies, nil
	}

	return "", errors.New("no cookies captured")
}

// IsBrowserLoginSupported returns true on macOS.
func IsBrowserLoginSupported() bool {
	return true
}
