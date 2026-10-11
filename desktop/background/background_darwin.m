//go:build darwin && !ios && cgo

#import <AppKit/AppKit.h>
#import <CoreServices/CoreServices.h>
#import <ServiceManagement/ServiceManagement.h>
#include <stdlib.h>
#include <string.h>
#include "background_darwin.h"

enum {
    loginItemUnavailable = -1,
    loginItemNotRegistered = 0,
    loginItemEnabled = 1,
    loginItemRequiresApproval = 2,
};

static BOOL isLaunchedAsLoginItem = NO;
static id launchObserver = nil;

static BOOL isInsideAppBundle(void) {
    return [[[NSBundle mainBundle] bundlePath] hasSuffix:@".app"] && [[NSBundle mainBundle] bundleIdentifier] != nil;
}

static void copyErrorMessage(NSError *error, char **errorMessage) {
    if (error != nil && errorMessage != NULL) {
        *errorMessage = strdup([[error localizedDescription] UTF8String]);
    }
}

int hisarLoginItemGetStatus(void) {
    if (!isInsideAppBundle()) {
        return loginItemUnavailable;
    }
    if (@available(macOS 13.0, *)) {
        switch ([SMAppService mainAppService].status) {
        case SMAppServiceStatusEnabled:
            return loginItemEnabled;
        case SMAppServiceStatusRequiresApproval:
            return loginItemRequiresApproval;
        default:
            return loginItemNotRegistered;
        }
    }
    return loginItemUnavailable;
}

int hisarLoginItemRegister(char **errorMessage) {
    if (!isInsideAppBundle()) {
        return 0;
    }
    if (@available(macOS 13.0, *)) {
        NSError *error = nil;
        if ([[SMAppService mainAppService] registerAndReturnError:&error]) {
            return 1;
        }
        // The user turned Hisar off in System Settings; only they can turn it on there.
        if (error.code == kSMErrorLaunchDeniedByUser) {
            return 1;
        }
        copyErrorMessage(error, errorMessage);
        return -1;
    }
    return 0;
}

int hisarLoginItemUnregister(char **errorMessage) {
    if (!isInsideAppBundle()) {
        return 1;
    }
    if (@available(macOS 13.0, *)) {
        SMAppService *service = [SMAppService mainAppService];
        if (service.status == SMAppServiceStatusNotRegistered || service.status == SMAppServiceStatusNotFound) {
            return 1;
        }
        NSError *error = nil;
        if ([service unregisterAndReturnError:&error]) {
            return 1;
        }
        copyErrorMessage(error, errorMessage);
        return -1;
    }
    return 1;
}

int hisarLoginItemOpenSystemSettings(void) {
    if (@available(macOS 13.0, *)) {
        dispatch_async(dispatch_get_main_queue(), ^{
            [SMAppService openSystemSettingsLoginItems];
        });
        return 1;
    }
    return 0;
}

static void recordLaunchEvent(void) {
    NSAppleEventDescriptor *event = [[NSAppleEventManager sharedAppleEventManager] currentAppleEvent];
    if (event == nil || event.eventClass != kCoreEventClass || event.eventID != kAEOpenApplication) {
        return;
    }
    if ([event paramDescriptorForKeyword:keyAEPropData].enumCodeValue == keyAELaunchedAsLogInItem) {
        isLaunchedAsLoginItem = YES;
    }
}

// The launch Apple event is only current while AppKit delivers the launch notifications, so the block runs synchronously.
void hisarStartLaunchObserver(void) {
    if (launchObserver != nil) {
        return;
    }
    NSNotificationCenter *center = [NSNotificationCenter defaultCenter];
    [center addObserverForName:NSApplicationWillFinishLaunchingNotification object:nil queue:nil usingBlock:^(NSNotification *notification) {
        recordLaunchEvent();
    }];
    launchObserver = [center addObserverForName:NSApplicationDidFinishLaunchingNotification object:nil queue:nil usingBlock:^(NSNotification *notification) {
        recordLaunchEvent();
    }];
}

int hisarWasLaunchedAsLoginItem(void) {
    return isLaunchedAsLoginItem ? 1 : 0;
}

void hisarSetDockIconVisible(int isVisible) {
    dispatch_block_t apply = ^{
        NSApplicationActivationPolicy policy = isVisible ? NSApplicationActivationPolicyRegular : NSApplicationActivationPolicyAccessory;
        if ([NSApp activationPolicy] != policy) {
            [NSApp setActivationPolicy:policy];
        }
        if (isVisible) {
            [NSApp activateIgnoringOtherApps:YES];
        }
    };
    if ([NSThread isMainThread]) {
        apply();
    } else {
        dispatch_async(dispatch_get_main_queue(), apply);
    }
}

void hisarSetAppAppearance(int appearance) {
    dispatch_block_t apply = ^{
        if (appearance == 1) {
            NSApp.appearance = [NSAppearance appearanceNamed:NSAppearanceNameAqua];
        } else if (appearance == 2) {
            NSApp.appearance = [NSAppearance appearanceNamed:NSAppearanceNameDarkAqua];
        } else {
            NSApp.appearance = nil;
        }
    };
    if ([NSThread isMainThread]) {
        apply();
    } else {
        dispatch_async(dispatch_get_main_queue(), apply);
    }
}
