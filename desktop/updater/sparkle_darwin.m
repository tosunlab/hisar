//go:build darwin && cgo && sparkle

#import <AppKit/AppKit.h>
#import <Sparkle/Sparkle.h>
#include "sparkle_darwin.h"

@interface HisarUpdaterDelegate : NSObject <SPUUpdaterDelegate>
@end

static SPUStandardUpdaterController *updaterController = nil;
static HisarUpdaterDelegate *updaterDelegate = nil;
static NSString *currentFeedURL = nil;
static NSString *currentAuthorization = nil;
static BOOL isAutomaticCheckEnabled = YES;

static void runOnMainThread(dispatch_block_t block) {
    if ([NSThread isMainThread]) {
        block();
    } else {
        dispatch_async(dispatch_get_main_queue(), block);
    }
}

static NSDictionary<NSString *, NSString *> *copyHTTPHeaders(void) {
    @synchronized ([HisarUpdaterDelegate class]) {
        return currentAuthorization == nil ? nil : @{@"Authorization": currentAuthorization};
    }
}

@implementation HisarUpdaterDelegate

- (nullable NSString *)feedURLStringForUpdater:(SPUUpdater *)updater {
    @synchronized ([HisarUpdaterDelegate class]) {
        return currentFeedURL;
    }
}

// Signed out, there is no server to ask; a manual check shows this message.
- (BOOL)updater:(SPUUpdater *)updater mayPerformUpdateCheck:(SPUUpdateCheck)updateCheck error:(NSError * __autoreleasing *)error {
    NSDictionary<NSString *, NSString *> *headers = copyHTTPHeaders();
    if (headers == nil || [self feedURLStringForUpdater:updater] == nil) {
        if (error != NULL) {
            *error = [NSError errorWithDomain:@"com.tryhisar.hisar.updates" code:1 userInfo:@{NSLocalizedDescriptionKey: @"Sign in to Hisar to check for updates."}];
        }
        return NO;
    }
    updater.httpHeaders = headers;
    return YES;
}

@end

void hisarUpdaterStart(void) {
    runOnMainThread(^{
        if (updaterController != nil) {
            return;
        }
        updaterDelegate = [[HisarUpdaterDelegate alloc] init];
        updaterController = [[SPUStandardUpdaterController alloc] initWithStartingUpdater:NO updaterDelegate:updaterDelegate userDriverDelegate:nil];
        updaterController.updater.httpHeaders = copyHTTPHeaders();
        updaterController.updater.automaticallyChecksForUpdates = isAutomaticCheckEnabled;
        [updaterController startUpdater];
    });
}

void hisarUpdaterSetSession(const char *feedURL, const char *authorization) {
    NSString *feedURLText = feedURL[0] == 0 ? nil : [NSString stringWithUTF8String:feedURL];
    NSString *authorizationText = authorization[0] == 0 ? nil : [NSString stringWithUTF8String:authorization];
    @synchronized ([HisarUpdaterDelegate class]) {
        currentFeedURL = feedURLText;
        currentAuthorization = authorizationText;
    }
    runOnMainThread(^{
        if (updaterController != nil) {
            updaterController.updater.httpHeaders = copyHTTPHeaders();
        }
    });
}

void hisarUpdaterSetAutomaticCheck(int isEnabled) {
    runOnMainThread(^{
        isAutomaticCheckEnabled = isEnabled != 0;
        if (updaterController != nil) {
            updaterController.updater.automaticallyChecksForUpdates = isAutomaticCheckEnabled;
        }
    });
}

int hisarUpdaterCheckForUpdates(void) {
    if (updaterController == nil) {
        return 0;
    }
    runOnMainThread(^{
        [updaterController checkForUpdates:nil];
    });
    return 1;
}
