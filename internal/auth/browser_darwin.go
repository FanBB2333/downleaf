//go:build darwin
// +build darwin

package auth

/*
#cgo CFLAGS: -x objective-c
#cgo LDFLAGS: -framework Cocoa -framework WebKit

#import <Cocoa/Cocoa.h>
#import <WebKit/WebKit.h>
#import <objc/runtime.h>

// Result structure passed back to Go
typedef struct {
    char* cookies;  // Semicolon-separated cookie string, or NULL on error
    char* error;    // Error message, or NULL on success
} BrowserLoginResult;

// Forward declaration
BrowserLoginResult runBrowserLogin(const char* siteURL, const char* loginPath, const char* successPath, int timeoutSeconds);

static const void* kLoginDelegateKey = &kLoginDelegateKey;
static const void* kWindowDelegateKey = &kWindowDelegateKey;

// Delegate to monitor navigation and extract cookies
@interface LoginDelegate : NSObject <WKNavigationDelegate, WKUIDelegate>
@property (nonatomic, strong) NSString* siteURL;
@property (nonatomic, strong) NSString* successPath;
@property (nonatomic, assign) BOOL completed;
@property (nonatomic, strong) NSWindow* window;
@property (nonatomic, strong) WKWebView* webView;
@property (nonatomic, assign) BrowserLoginResult* result;
@property (nonatomic, strong) id semaphore;
- (void)finishWithCookies:(NSString*)cookies error:(NSString*)errorMsg;
@end

@implementation LoginDelegate

- (void)finishWithCookies:(NSString*)cookies error:(NSString*)errorMsg {
    if (self.completed) return;

    self.completed = YES;

    if (errorMsg != nil) {
        self.result->error = strdup([errorMsg UTF8String]);
    } else if (cookies != nil) {
        self.result->cookies = strdup([cookies UTF8String]);
    } else {
        self.result->error = strdup("Unknown error: no cookies captured");
    }

    dispatch_async(dispatch_get_main_queue(), ^{
        [self.window close];
    });
    dispatch_semaphore_signal((dispatch_semaphore_t)self.semaphore);
}

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

            [self finishWithCookies:[parts componentsJoinedByString:@"; "] error:nil];
        }];
    }
}

- (void)webView:(WKWebView *)webView didFailNavigation:(WKNavigation *)navigation withError:(NSError *)error {
    [self finishWithCookies:nil error:[error localizedDescription]];
}

- (void)webView:(WKWebView *)webView didFailProvisionalNavigation:(WKNavigation *)navigation withError:(NSError *)error {
    // Ignore cancellation errors (happen during redirects)
    if (error.code == NSURLErrorCancelled) return;

    [self finishWithCookies:nil error:[error localizedDescription]];
}

- (nullable WKWebView *)webView:(WKWebView *)webView
    createWebViewWithConfiguration:(WKWebViewConfiguration *)configuration
    forNavigationAction:(WKNavigationAction *)navigationAction
    windowFeatures:(WKWindowFeatures *)windowFeatures {
    if (!navigationAction.targetFrame || !navigationAction.targetFrame.isMainFrame) {
        [webView loadRequest:navigationAction.request];
    }
    return nil;
}

- (void)webViewWebContentProcessDidTerminate:(WKWebView *)webView {
    [self finishWithCookies:nil error:@"Web content process terminated unexpectedly"];
}

@end

// Window delegate to handle close button
@interface WindowCloseDelegate : NSObject <NSWindowDelegate>
@property (nonatomic, unsafe_unretained) LoginDelegate* loginDelegate;
@end

@implementation WindowCloseDelegate
- (BOOL)windowShouldClose:(NSWindow *)sender {
    if (!self.loginDelegate.completed) {
        [self.loginDelegate finishWithCookies:nil error:@"Login cancelled by user"];
    }
    return YES;
}
@end

BrowserLoginResult runBrowserLogin(const char* siteURL, const char* loginPath, const char* successPath, int timeoutSeconds) {
    __block BrowserLoginResult result = {NULL, NULL};
    dispatch_semaphore_t semaphore = dispatch_semaphore_create(0);
    __block NSWindow* window = nil;

    dispatch_sync(dispatch_get_main_queue(), ^{
        @autoreleasepool {
            [NSApplication sharedApplication];

            NSString* siteURLStr = [NSString stringWithUTF8String:siteURL];
            NSString* loginPathStr = [NSString stringWithUTF8String:loginPath];
            NSString* successPathStr = [NSString stringWithUTF8String:successPath];

            NSRect frame = NSMakeRect(0, 0, 980, 760);
            window = [[NSWindow alloc] initWithContentRect:frame
                                                 styleMask:(NSWindowStyleMaskTitled |
                                                           NSWindowStyleMaskClosable |
                                                           NSWindowStyleMaskResizable |
                                                           NSWindowStyleMaskMiniaturizable)
                                                   backing:NSBackingStoreBuffered
                                                     defer:NO];
            [window setTitle:@"Login to Overleaf"];
            [window center];
            [window setReleasedWhenClosed:NO];

            NSView* contentView = [[NSView alloc] initWithFrame:[[window contentView] bounds]];
            [contentView setAutoresizingMask:NSViewWidthSizable | NSViewHeightSizable];
            [window setContentView:contentView];

            WKWebViewConfiguration* config = [[WKWebViewConfiguration alloc] init];
            config.websiteDataStore = [WKWebsiteDataStore defaultDataStore];

            WKWebView* webView = [[WKWebView alloc] initWithFrame:[contentView bounds] configuration:config];
            [webView setAutoresizingMask:NSViewWidthSizable | NSViewHeightSizable];
            [contentView addSubview:webView];

            LoginDelegate* delegate = [[LoginDelegate alloc] init];
            delegate.siteURL = siteURLStr;
            delegate.successPath = successPathStr;
            delegate.completed = NO;
            delegate.window = window;
            delegate.webView = webView;
            delegate.result = &result;
            delegate.semaphore = semaphore;
            webView.navigationDelegate = delegate;
            webView.UIDelegate = delegate;

            WindowCloseDelegate* windowDelegate = [[WindowCloseDelegate alloc] init];
            windowDelegate.loginDelegate = delegate;
            window.delegate = windowDelegate;

            objc_setAssociatedObject(window, kLoginDelegateKey, delegate, OBJC_ASSOCIATION_RETAIN_NONATOMIC);
            objc_setAssociatedObject(window, kWindowDelegateKey, windowDelegate, OBJC_ASSOCIATION_RETAIN_NONATOMIC);

            NSString* fullURL = [siteURLStr stringByAppendingString:loginPathStr];
            NSURL* url = [NSURL URLWithString:fullURL];
            NSURLRequest* request = [NSURLRequest requestWithURL:url];
            [webView loadRequest:request];

            [window makeKeyAndOrderFront:nil];
            [NSApp activateIgnoringOtherApps:YES];

            dispatch_after(dispatch_time(DISPATCH_TIME_NOW, (int64_t)(timeoutSeconds * NSEC_PER_SEC)),
                          dispatch_get_main_queue(), ^{
                [delegate finishWithCookies:nil error:@"Login timed out"];
            });
        }
    });

    dispatch_semaphore_wait(semaphore, DISPATCH_TIME_FOREVER);

    dispatch_sync(dispatch_get_main_queue(), ^{
        if (window != nil) {
            objc_setAssociatedObject(window, kLoginDelegateKey, nil, OBJC_ASSOCIATION_RETAIN_NONATOMIC);
            objc_setAssociatedObject(window, kWindowDelegateKey, nil, OBJC_ASSOCIATION_RETAIN_NONATOMIC);
        }
    });

    return result;
}
*/
import "C"

import (
	"errors"
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
