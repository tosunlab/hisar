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
static LAContext *authenticatedContext = nil;
static NSString *preparedAccount = nil;
static NSString *authenticatedAccount = nil;

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

void hisarBiometricPrepareLoad(const char *account) {
    @synchronized ([LAContext class]) {
        [authenticatedContext invalidate];
        authenticatedContext = nil;
        authenticatedAccount = nil;
        preparedAccount = account == NULL ? nil : [NSString stringWithUTF8String:account];
    }
}

int hisarBiometricAuthenticate(const char *reason, void *window, int embed, int controlSize, double x, double y, double width, double height, char **errorMessage) {
    NSString *account = nil;
    @synchronized ([LAContext class]) {
        account = preparedAccount;
        preparedAccount = nil;
    }
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
        @synchronized ([LAContext class]) {
            if (account != nil) {
                authenticatedContext = context;
                authenticatedAccount = account;
            } else {
                [context invalidate];
            }
        }
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
        [authenticatedContext invalidate];
        authenticatedContext = nil;
        authenticatedAccount = nil;
        preparedAccount = nil;
    }
}

static NSMutableDictionary *createLegacyKeychainQuery(const char *service, const char *account) {
    return [@{
        (__bridge id)kSecClass: (__bridge id)kSecClassGenericPassword,
        (__bridge id)kSecAttrService: [NSString stringWithUTF8String:service],
        (__bridge id)kSecAttrAccount: [NSString stringWithUTF8String:account],
    } mutableCopy];
}

static NSMutableDictionary *createKeychainQuery(const char *service, const char *account) {
    NSMutableDictionary *query = createLegacyKeychainQuery(service, account);
    query[(__bridge id)kSecAttrService] = [[NSString stringWithUTF8String:service] stringByAppendingString:@".biometry-current-set.v1"];
    query[(__bridge id)kSecUseDataProtectionKeychain] = @YES;
    return query;
}

int hisarKeychainStore(const char *service, const char *account, const void *data, int length) {
    CFErrorRef error = NULL;
    SecAccessControlRef access = SecAccessControlCreateWithFlags(NULL,
        kSecAttrAccessibleWhenUnlockedThisDeviceOnly, kSecAccessControlBiometryCurrentSet, &error);
    if (access == NULL) {
        if (error != NULL) CFRelease(error);
        return errSecParam;
    }
    NSMutableDictionary *query = createKeychainQuery(service, account);
    NSData *value = [NSData dataWithBytes:data length:length];
    query[(__bridge id)kSecAttrAccessControl] = (__bridge id)access;
    query[(__bridge id)kSecValueData] = value;
    query[(__bridge id)kSecAttrLabel] = @"Hisar Touch ID unlock";
    OSStatus status = SecItemAdd((__bridge CFDictionaryRef)query, NULL);
    CFRelease(access);
    if (status == errSecDuplicateItem) {
        status = SecItemUpdate((__bridge CFDictionaryRef)createKeychainQuery(service, account),
            (__bridge CFDictionaryRef)@{(__bridge id)kSecValueData: value});
    }
    return (int)status;
}

static int loadKey(NSMutableDictionary *query, LAContext *context, void **data, int *length) {
    if (context != nil) {
        context.localizedFallbackTitle = @"";
        query[(__bridge id)kSecUseAuthenticationContext] = context;
        query[(__bridge id)kSecUseOperationPrompt] = @"unlock Hisar";
    }
    query[(__bridge id)kSecReturnData] = @YES;
    query[(__bridge id)kSecMatchLimit] = (__bridge id)kSecMatchLimitOne;
    CFTypeRef result = NULL;
    OSStatus status = SecItemCopyMatching((__bridge CFDictionaryRef)query, &result);
    if (context != nil) [context invalidate];
    if (status != errSecSuccess) return (int)status;
    NSData *value = (__bridge_transfer NSData *)result;
    *length = (int)value.length;
    *data = malloc(value.length);
    memcpy(*data, value.bytes, value.length);
    return 0;
}

int hisarKeychainLoad(const char *service, const char *account, int freshContext, void **data, int *length) {
    LAContext *context = nil;
    @synchronized ([LAContext class]) {
        if (!freshContext && [authenticatedAccount isEqualToString:[NSString stringWithUTF8String:account]]) context = authenticatedContext;
        if (context != authenticatedContext) [authenticatedContext invalidate];
        authenticatedContext = nil;
        authenticatedAccount = nil;
    }
    if (context == nil) context = [[LAContext alloc] init];
    return loadKey(createKeychainQuery(service, account), context, data, length);
}

int hisarKeychainLoadLegacy(const char *service, const char *account, void **data, int *length) {
    return loadKey(createLegacyKeychainQuery(service, account), nil, data, length);
}

int hisarKeychainHasLegacy(const char *service, const char *account) {
    NSMutableDictionary *query = createLegacyKeychainQuery(service, account);
    query[(__bridge id)kSecUseAuthenticationUI] = (__bridge id)kSecUseAuthenticationUIFail;
    return SecItemCopyMatching((__bridge CFDictionaryRef)query, NULL) != errSecItemNotFound;
}

int hisarKeychainHasProtected(const char *service, const char *account) {
    NSMutableDictionary *query = createKeychainQuery(service, account);
    query[(__bridge id)kSecUseAuthenticationUI] = (__bridge id)kSecUseAuthenticationUIFail;
    OSStatus status = SecItemCopyMatching((__bridge CFDictionaryRef)query, NULL);
    return status == errSecSuccess || status == errSecInteractionNotAllowed;
}

int hisarKeychainDeleteLegacy(const char *service, const char *account) {
    OSStatus status = SecItemDelete((__bridge CFDictionaryRef)createLegacyKeychainQuery(service, account));
    return status == errSecItemNotFound ? 0 : (int)status;
}

int hisarKeychainDelete(const char *service, const char *account) {
    OSStatus status = SecItemDelete((__bridge CFDictionaryRef)createKeychainQuery(service, account));
    return status == errSecItemNotFound ? 0 : (int)status;
}
