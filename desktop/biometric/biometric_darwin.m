//go:build darwin && cgo

#import <AppKit/AppKit.h>
#import <LocalAuthentication/LocalAuthentication.h>
#import <LocalAuthenticationEmbeddedUI/LocalAuthenticationEmbeddedUI.h>
#import <Security/Security.h>
#include <stdlib.h>
#include <string.h>
#include "biometric_darwin.h"

enum {
    hisarBiometricSuccess = 0,
    hisarBiometricFailed = 1,
    hisarBiometricCancelled = 2,
    hisarBiometricUnavailable = 3,
};

static LAContext *activeContext = nil;
static LAAuthenticationView *activeView = nil;

int hisarBiometricIsAvailable(void) {
    LAContext *context = [[LAContext alloc] init];
    return [context canEvaluatePolicy:LAPolicyDeviceOwnerAuthenticationWithBiometrics error:nil] ? 1 : 0;
}

static NSRect convertToContentRect(NSView *contentView, double x, double y, double width, double height) {
    if (contentView.isFlipped) {
        return NSMakeRect(x, y, width, height);
    }
    return NSMakeRect(x, contentView.bounds.size.height - y - height, width, height);
}

// The view keeps its own size for each control size, so it is centred on the slot instead of stretched into it.
static void placeAuthenticationView(NSView *view, NSView *contentView, double x, double y, double width, double height) {
    NSRect slot = convertToContentRect(contentView, x, y, width, height);
    NSSize size = view.fittingSize;
    if (size.width <= 0 || size.height <= 0) {
        view.frame = slot;
        return;
    }
    view.frame = NSMakeRect(round(NSMidX(slot) - size.width / 2), round(NSMidY(slot) - size.height / 2), size.width, size.height);
}

static void runOnMainThread(dispatch_block_t block) {
    if ([NSThread isMainThread]) {
        block();
    } else {
        dispatch_sync(dispatch_get_main_queue(), block);
    }
}

int hisarBiometricAuthenticate(const char *reason, void *window, int embed, int controlSize, double x, double y, double width, double height, char **errorMessage) {
    LAContext *context = [[LAContext alloc] init];
    context.localizedFallbackTitle = @"";
    NSError *availabilityError = nil;
    if (![context canEvaluatePolicy:LAPolicyDeviceOwnerAuthenticationWithBiometrics error:&availabilityError]) {
        return hisarBiometricUnavailable;
    }

    // The embedded view makes macOS show Touch ID inside our window instead of the system dialog.
    if (embed && window != NULL) {
        runOnMainThread(^{
            NSWindow *nativeWindow = (__bridge NSWindow *)window;
            NSView *contentView = nativeWindow.contentView;
            LAAuthenticationView *authenticationView = [[LAAuthenticationView alloc] initWithContext:context controlSize:(NSControlSize)controlSize];
            [contentView addSubview:authenticationView positioned:NSWindowAbove relativeTo:nil];
            placeAuthenticationView(authenticationView, contentView, x, y, width, height);
            activeView = authenticationView;
        });
    }
    @synchronized ([LAContext class]) {
        activeContext = context;
    }

    dispatch_semaphore_t finished = dispatch_semaphore_create(0);
    __block BOOL isSuccess = NO;
    __block NSError *evaluationError = nil;
    [context evaluatePolicy:LAPolicyDeviceOwnerAuthenticationWithBiometrics
            localizedReason:[NSString stringWithUTF8String:reason]
                      reply:^(BOOL success, NSError *error) {
        isSuccess = success;
        evaluationError = error;
        dispatch_semaphore_signal(finished);
    }];
    dispatch_semaphore_wait(finished, DISPATCH_TIME_FOREVER);

    @synchronized ([LAContext class]) {
        activeContext = nil;
    }
    runOnMainThread(^{
        [activeView removeFromSuperview];
        activeView = nil;
    });

    if (isSuccess) {
        return hisarBiometricSuccess;
    }
    if (evaluationError != nil) {
        if (errorMessage != NULL) {
            *errorMessage = strdup(evaluationError.localizedDescription.UTF8String);
        }
        switch (evaluationError.code) {
            case LAErrorUserCancel:
            case LAErrorAppCancel:
            case LAErrorSystemCancel:
            case LAErrorUserFallback:
                return hisarBiometricCancelled;
        }
    }
    return hisarBiometricFailed;
}

void hisarBiometricMovePrompt(double x, double y, double width, double height) {
    dispatch_async(dispatch_get_main_queue(), ^{
        if (activeView != nil && activeView.superview != nil) {
            placeAuthenticationView(activeView, activeView.superview, x, y, width, height);
        }
    });
}

void hisarBiometricCancel(void) {
    @synchronized ([LAContext class]) {
        [activeContext invalidate];
    }
}

static NSMutableDictionary *createKeychainQuery(const char *service, const char *account) {
    return [@{
        (__bridge id)kSecClass: (__bridge id)kSecClassGenericPassword,
        (__bridge id)kSecAttrService: [NSString stringWithUTF8String:service],
        (__bridge id)kSecAttrAccount: [NSString stringWithUTF8String:account],
    } mutableCopy];
}

int hisarKeychainStore(const char *service, const char *account, const void *data, int length) {
    NSMutableDictionary *query = createKeychainQuery(service, account);
    SecItemDelete((__bridge CFDictionaryRef)query);
    query[(__bridge id)kSecValueData] = [NSData dataWithBytes:data length:length];
    query[(__bridge id)kSecAttrAccessible] = (__bridge id)kSecAttrAccessibleWhenUnlockedThisDeviceOnly;
    query[(__bridge id)kSecAttrLabel] = @"Hisar Touch ID unlock";
    return (int)SecItemAdd((__bridge CFDictionaryRef)query, NULL);
}

int hisarKeychainLoad(const char *service, const char *account, void **data, int *length) {
    NSMutableDictionary *query = createKeychainQuery(service, account);
    query[(__bridge id)kSecReturnData] = @YES;
    query[(__bridge id)kSecMatchLimit] = (__bridge id)kSecMatchLimitOne;
    CFTypeRef result = NULL;
    OSStatus status = SecItemCopyMatching((__bridge CFDictionaryRef)query, &result);
    if (status != errSecSuccess) {
        return (int)status;
    }
    NSData *value = (__bridge_transfer NSData *)result;
    *length = (int)value.length;
    *data = malloc(value.length);
    memcpy(*data, value.bytes, value.length);
    return 0;
}

int hisarKeychainDelete(const char *service, const char *account) {
    OSStatus status = SecItemDelete((__bridge CFDictionaryRef)createKeychainQuery(service, account));
    return status == errSecItemNotFound ? 0 : (int)status;
}
