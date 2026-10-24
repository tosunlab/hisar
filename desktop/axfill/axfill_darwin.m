#import <AppKit/AppKit.h>
#import <ApplicationServices/ApplicationServices.h>
#include <os/lock.h>
#include <time.h>
#include "axfill_darwin.h"

extern AXError _AXUIElementGetWindow(AXUIElementRef element, CGWindowID *windowID);

static const int maximumDepth = 120;
static const int maximumNodes = 40000;
static const int promptDepth = 12;
static const int maximumPromptNodes = 2000;
static const NSTimeInterval forgetAfter = 600;
static const double messagingTimeout = 1.0;
static const CGKeyCode keyReturn = 36;
static NSString *const webAreaRole = @"AXWebArea";

// One exported call is one operation on the calling thread: it sends no accessibility message once its limit ends, and
// each message's timeout is what is left. The accessibility API keeps that timeout, so a message can outlast it.
static _Thread_local uint64_t operationDeadline;
static _Thread_local const int32_t *operationCancelled;
static _Thread_local BOOL isOperationCut;
static _Thread_local BOOL isMessagingFailed;

static void beginOperation(AXFillLimit limit) {
    operationDeadline = clock_gettime_nsec_np(CLOCK_UPTIME_RAW) + (uint64_t)MAX(limit.milliseconds, 0) * NSEC_PER_MSEC;
    operationCancelled = limit.cancelled;
    isOperationCut = NO;
    isMessagingFailed = NO;
}

// minimumMessageSeconds is the least time left worth a message: a smaller timeout may round to zero, which the
// accessibility API takes as its default.
static const double minimumMessageSeconds = 0.01;

static double readRemainingSeconds(void) {
    uint64_t now = clock_gettime_nsec_np(CLOCK_UPTIME_RAW);
    double remaining = now < operationDeadline ? (double)(operationDeadline - now) / NSEC_PER_SEC : 0;
    if ((operationCancelled != NULL && __atomic_load_n(operationCancelled, __ATOMIC_SEQ_CST) != 0) || remaining < minimumMessageSeconds) {
        isOperationCut = YES;
        return 0;
    }
    return remaining;
}

// prepareMessage sets the timeout of the next message to element to what is left of the operation, and refuses once
// it is over: a timeout set on the application element does not cover the elements read from it.
static BOOL prepareMessage(AXUIElementRef element) {
    double remaining = readRemainingSeconds();
    if (remaining <= 0) {
        return NO;
    }
    AXUIElementSetMessagingTimeout(element, (float)MIN(remaining, messagingTimeout));
    return YES;
}

static BOOL pauseWithin(double seconds) {
    double remaining = readRemainingSeconds();
    if (remaining <= 0) {
        return NO;
    }
    usleep((useconds_t)(MIN(seconds, remaining) * USEC_PER_SEC));
    return readRemainingSeconds() > 0;
}

static int endOperation(int result) {
    return isOperationCut ? axfillUnreadable : result;
}

// The registry gives every element a snapshot reads a number that stays the same for as long as the element exists, so
// a later snapshot can tell the same document or field from a new one.
static os_unfair_lock registryLock = OS_UNFAIR_LOCK_INIT;
static CFMutableDictionaryRef identifiersByElement;
static NSMutableDictionary<NSNumber *, id> *elementsByIdentifier;
static NSMutableDictionary<NSNumber *, NSDate *> *lastSeenByIdentifier;
static uint64_t nextIdentifier = 1;
static NSMutableSet<NSNumber *> *readyProcesses;
static NSMutableDictionary<NSNumber *, NSDate *> *promptShownAt;
static NSMutableDictionary<NSNumber *, NSDate *> *promptPressedAt;

static void ensureRegistry(void) {
    if (identifiersByElement == NULL) {
        identifiersByElement = CFDictionaryCreateMutable(NULL, 0, &kCFTypeDictionaryKeyCallBacks, &kCFTypeDictionaryValueCallBacks);
        elementsByIdentifier = [NSMutableDictionary dictionary];
        lastSeenByIdentifier = [NSMutableDictionary dictionary];
        readyProcesses = [NSMutableSet set];
        promptShownAt = [NSMutableDictionary dictionary];
        promptPressedAt = [NSMutableDictionary dictionary];
    }
}

static uint64_t registerElement(AXUIElementRef element) {
    os_unfair_lock_lock(&registryLock);
    ensureRegistry();
    NSNumber *identifier = (__bridge NSNumber *)CFDictionaryGetValue(identifiersByElement, element);
    if (identifier == nil) {
        identifier = @(nextIdentifier++);
        CFDictionarySetValue(identifiersByElement, element, (__bridge const void *)identifier);
        elementsByIdentifier[identifier] = (__bridge id)element;
    }
    lastSeenByIdentifier[identifier] = [NSDate date];
    os_unfair_lock_unlock(&registryLock);
    return identifier.unsignedLongLongValue;
}

static AXUIElementRef copyRegisteredElement(uint64_t identifier) {
    os_unfair_lock_lock(&registryLock);
    ensureRegistry();
    AXUIElementRef element = (__bridge AXUIElementRef)elementsByIdentifier[@(identifier)];
    if (element != NULL) {
        CFRetain(element);
    }
    os_unfair_lock_unlock(&registryLock);
    return element;
}

static void forgetOldElements(void) {
    os_unfair_lock_lock(&registryLock);
    ensureRegistry();
    NSDate *cutoff = [NSDate dateWithTimeIntervalSinceNow:-forgetAfter];
    for (NSNumber *identifier in lastSeenByIdentifier.allKeys) {
        if ([lastSeenByIdentifier[identifier] compare:cutoff] == NSOrderedAscending) {
            CFDictionaryRemoveValue(identifiersByElement, (__bridge AXUIElementRef)elementsByIdentifier[identifier]);
            [elementsByIdentifier removeObjectForKey:identifier];
            [lastSeenByIdentifier removeObjectForKey:identifier];
            [promptShownAt removeObjectForKey:identifier];
            [promptPressedAt removeObjectForKey:identifier];
        }
    }
    os_unfair_lock_unlock(&registryLock);
}

static BOOL isProcessReady(int pid) {
    os_unfair_lock_lock(&registryLock);
    ensureRegistry();
    BOOL isReady = [readyProcesses containsObject:@(pid)];
    os_unfair_lock_unlock(&registryLock);
    return isReady;
}

static void notePromptShown(uint64_t prompt) {
    os_unfair_lock_lock(&registryLock);
    ensureRegistry();
    if (promptShownAt[@(prompt)] == nil) {
        promptShownAt[@(prompt)] = [NSDate date];
    }
    os_unfair_lock_unlock(&registryLock);
}

// Chromium ignores bubble presses during the double-click interval after appearance
// or the last press, to reject clicks intended for the content underneath.
static BOOL waitUntilPressable(uint64_t prompt) {
    NSTimeInterval interval = NSEvent.doubleClickInterval + 0.1;
    os_unfair_lock_lock(&registryLock);
    ensureRegistry();
    NSDate *ready = [promptShownAt[@(prompt)] ?: [NSDate date] dateByAddingTimeInterval:interval];
    NSDate *pressed = promptPressedAt[@(prompt)];
    os_unfair_lock_unlock(&registryLock);
    if (pressed != nil) {
        ready = [ready laterDate:[pressed dateByAddingTimeInterval:interval]];
    }
    NSTimeInterval wait = [ready timeIntervalSinceNow];
    if (wait <= 0) {
        return YES;
    }
    return wait < readRemainingSeconds() && pauseWithin(wait);
}

static void notePromptPressed(uint64_t prompt) {
    os_unfair_lock_lock(&registryLock);
    ensureRegistry();
    promptPressedAt[@(prompt)] = [NSDate date];
    os_unfair_lock_unlock(&registryLock);
}

static void setProcessReady(int pid) {
    os_unfair_lock_lock(&registryLock);
    ensureRegistry();
    [readyProcesses addObject:@(pid)];
    os_unfair_lock_unlock(&registryLock);
}

static id copyAttribute(AXUIElementRef element, CFStringRef name) {
    CFTypeRef value = NULL;
    if (!prepareMessage(element)) {
        return nil;
    }
    AXError error = AXUIElementCopyAttributeValue(element, name, &value);
    if (error != kAXErrorSuccess) {
        isMessagingFailed = isMessagingFailed || error == kAXErrorCannotComplete;
        return nil;
    }
    return CFBridgingRelease(value);
}

static NSString *copyString(AXUIElementRef element, CFStringRef name) {
    id value = copyAttribute(element, name);
    return [value isKindOfClass:NSString.class] ? value : nil;
}

static NSURL *copyURL(AXUIElementRef element) {
    id value = copyAttribute(element, kAXURLAttribute);
    if ([value isKindOfClass:NSURL.class]) {
        return value;
    }
    if ([value isKindOfClass:NSString.class]) {
        return [NSURL URLWithString:value];
    }
    return nil;
}

static BOOL isElement(id value) {
    return value != nil && CFGetTypeID((__bridge CFTypeRef)value) == AXUIElementGetTypeID();
}

static NSArray *copyChildren(AXUIElementRef element) {
    id children = copyAttribute(element, kAXChildrenAttribute);
    return [children isKindOfClass:NSArray.class] ? children : @[];
}

static BOOL copyFrame(AXUIElementRef element, CGRect *frame) {
    id position = copyAttribute(element, kAXPositionAttribute);
    id size = copyAttribute(element, kAXSizeAttribute);
    if (position == nil || size == nil) {
        return NO;
    }
    return AXValueGetValue((__bridge AXValueRef)position, kAXValueTypeCGPoint, &frame->origin) &&
           AXValueGetValue((__bridge AXValueRef)size, kAXValueTypeCGSize, &frame->size);
}

static AXError setAttribute(AXUIElementRef element, CFStringRef name, CFTypeRef value) {
    return prepareMessage(element) ? AXUIElementSetAttributeValue(element, name, value) : kAXErrorCannotComplete;
}

static AXError setBoolean(AXUIElementRef element, CFStringRef name, BOOL value) {
    return setAttribute(element, name, value ? kCFBooleanTrue : kCFBooleanFalse);
}

static AXError press(AXUIElementRef element) {
    return prepareMessage(element) ? AXUIElementPerformAction(element, kAXPressAction) : kAXErrorCannotComplete;
}

static BOOL copyWindowID(AXUIElementRef window, CGWindowID *windowID) {
    return prepareMessage(window) && _AXUIElementGetWindow(window, windowID) == kAXErrorSuccess;
}

static NSString *originKey(NSURL *url) {
    if (url == nil) {
        return @"";
    }
    NSString *scheme = url.scheme.lowercaseString;
    NSString *host = url.host.lowercaseString;
    if (scheme.length == 0 || host.length == 0) {
        return url.absoluteString ?: @"";
    }
    return [NSString stringWithFormat:@"%@://%@:%@", scheme, host, url.port ?: @""];
}

static int countCharacters(AXUIElementRef element) {
    id count = copyAttribute(element, kAXNumberOfCharactersAttribute);
    if ([count isKindOfClass:NSNumber.class]) {
        return [count intValue];
    }
    NSString *value = copyString(element, kAXValueAttribute);
    return (int)value.length;
}

static NSString *copyLabel(AXUIElementRef element) {
    NSString *title = copyString(element, kAXTitleAttribute);
    if (title.length > 0) {
        return title;
    }
    NSString *description = copyString(element, kAXDescriptionAttribute);
    if (description.length > 0) {
        return description;
    }
    id titleElement = copyAttribute(element, kAXTitleUIElementAttribute);
    if (isElement(titleElement)) {
        AXUIElementRef label = (__bridge AXUIElementRef)titleElement;
        return copyString(label, kAXValueAttribute) ?: copyString(label, kAXTitleAttribute) ?: @"";
    }
    return @"";
}

static NSString *copyButtonTitle(AXUIElementRef element) {
    NSString *title = copyLabel(element);
    if (title.length > 0) {
        return title;
    }
    NSMutableArray *texts = [NSMutableArray array];
    for (id child in copyChildren(element)) {
        AXUIElementRef childElement = (__bridge AXUIElementRef)child;
        NSString *text = copyString(childElement, kAXValueAttribute) ?: copyString(childElement, kAXTitleAttribute);
        if (text.length > 0) {
            [texts addObject:text];
        }
    }
    return [texts componentsJoinedByString:@" "];
}

static BOOL isVisibleIn(AXUIElementRef element, CGRect area) {
    CGRect frame;
    if (!copyFrame(element, &frame) || frame.size.width < 8 || frame.size.height < 8) {
        return NO;
    }
    return CGRectContainsPoint(area, CGPointMake(CGRectGetMidX(frame), CGRectGetMidY(frame)));
}

static BOOL isEditable(AXUIElementRef element) {
    Boolean isSettable = false;
    if (!prepareMessage(element) || AXUIElementIsAttributeSettable(element, kAXValueAttribute, &isSettable) != kAXErrorSuccess || !isSettable) {
        return NO;
    }
    id isEnabled = copyAttribute(element, kAXEnabledAttribute);
    return ![isEnabled isKindOfClass:NSNumber.class] || [isEnabled boolValue];
}

@interface AXFillWalk : NSObject
@property(nonatomic) NSMutableArray *frames;
@property(nonatomic) NSMutableArray *fields;
@property(nonatomic) NSMutableArray *buttons;
@property(nonatomic) int nodes;
@property(nonatomic) CGRect topArea;
@property(nonatomic) BOOL isTopAreaEmpty;
@property(nonatomic) BOOL isIncomplete;
@end

@implementation AXFillWalk
@end

static void walkElement(AXUIElementRef element, NSInteger frame, int depth, AXFillWalk *walk) {
    if (isOperationCut) {
        return;
    }
    if (depth > maximumDepth || walk.nodes >= maximumNodes) {
        walk.isIncomplete = YES;
        return;
    }
    walk.nodes++;
    NSString *role = copyString(element, kAXRoleAttribute) ?: @"";
    NSArray *children = nil;
    if ([role isEqualToString:webAreaRole]) {
        NSURL *url = copyURL(element);
        [walk.frames addObject:@{
            @"parent" : @(frame),
            @"document" : @(registerElement(element)),
            @"url" : url.absoluteString ?: @"",
            @"origin" : originKey(url),
        }];
        children = copyChildren(element);
        if (walk.frames.count == 1) {
            CGRect area = CGRectNull;
            copyFrame(element, &area);
            walk.topArea = area;
            walk.isTopAreaEmpty = children.count == 0;
        }
        frame = (NSInteger)walk.frames.count - 1;
    } else if (frame >= 0 && ([role isEqualToString:(__bridge NSString *)kAXTextFieldRole] || [role isEqualToString:(__bridge NSString *)kAXComboBoxRole])) {
        NSString *subrole = copyString(element, kAXSubroleAttribute) ?: @"";
        [walk.fields addObject:@{
            @"id" : @(registerElement(element)),
            @"frame" : @(frame),
            @"isSecure" : @([subrole isEqualToString:(__bridge NSString *)kAXSecureTextFieldSubrole]),
            @"inputType" : copyString(element, kAXRoleDescriptionAttribute) ?: @"",
            @"label" : copyLabel(element),
            @"identifier" : copyString(element, CFSTR("AXDOMIdentifier")) ?: @"",
            @"placeholder" : copyString(element, kAXPlaceholderValueAttribute) ?: @"",
            @"isEditable" : @(isEditable(element)),
            @"isVisible" : @(isVisibleIn(element, walk.topArea)),
            @"length" : @(countCharacters(element)),
        }];
        return;
    } else if (frame >= 0 && [role isEqualToString:(__bridge NSString *)kAXButtonRole]) {
        [walk.buttons addObject:@{
            @"id" : @(registerElement(element)),
            @"frame" : @(frame),
            @"title" : copyButtonTitle(element),
            @"isVisible" : @(isVisibleIn(element, walk.topArea)),
        }];
        return;
    }
    for (id child in children ?: copyChildren(element)) {
        walkElement((__bridge AXUIElementRef)child, frame, depth + 1, walk);
    }
}

static AXUIElementRef copyWindow(AXUIElementRef app, uint32_t windowID) {
    for (id window in copyAttribute(app, kAXWindowsAttribute) ?: @[]) {
        CGWindowID mapped = 0;
        if (copyWindowID((__bridge AXUIElementRef)window, &mapped) && mapped == windowID) {
            return (AXUIElementRef)CFBridgingRetain(window);
        }
    }
    return NULL;
}

static int readWindowState(int pid, uint32_t windowID) {
    NSArray *windows = CFBridgingRelease(CGWindowListCopyWindowInfo(kCGWindowListOptionIncludingWindow, windowID));
    for (NSDictionary *window in windows) {
        if ([window[(__bridge NSString *)kCGWindowNumber] unsignedIntValue] != windowID) {
            continue;
        }
        if ([window[(__bridge NSString *)kCGWindowOwnerPID] intValue] != pid || [window[(__bridge NSString *)kCGWindowLayer] intValue] != 0) {
            return axfillWindowNotFound;
        }
        return [window[(__bridge NSString *)kCGWindowIsOnscreen] boolValue] ? axfillOK : axfillNotVisible;
    }
    return axfillWindowClosed;
}

int axfillReadWindowState(int pid, uint32_t windowID) {
    @autoreleasepool {
        return readWindowState(pid, windowID);
    }
}

static char *copyJSON(id object) {
    NSData *data = [NSJSONSerialization dataWithJSONObject:object options:0 error:nil];
    if (data == nil) {
        return NULL;
    }
    char *buffer = malloc(data.length + 1);
    memcpy(buffer, data.bytes, data.length);
    buffer[data.length] = 0;
    return buffer;
}

int axfillHasAccess(void) {
    return AXIsProcessTrusted();
}

void axfillRequestAccess(void) {
    @autoreleasepool {
        AXIsProcessTrustedWithOptions((__bridge CFDictionaryRef) @{(__bridge NSString *)kAXTrustedCheckOptionPrompt : @YES});
    }
}

char *axfillObserve(int pid, uint32_t windowID, int readyMilliseconds, AXFillLimit limit, int *status) {
    @autoreleasepool {
        beginOperation(limit);
        if (!AXIsProcessTrusted()) {
            *status = axfillNoAccess;
            return NULL;
        }
        *status = readWindowState(pid, windowID);
        if (*status != axfillOK) {
            return NULL;
        }
        forgetOldElements();
        AXUIElementRef app = AXUIElementCreateApplication(pid);
        NSDate *readyDeadline = [NSDate dateWithTimeIntervalSinceNow:readyMilliseconds / 1000.0];
        BOOL isEnabled = NO;
        AXFillWalk *walk = nil;
        AXUIElementRef window = NULL;
        while (YES) {
            isMessagingFailed = NO;
            if (window == NULL) {
                window = copyWindow(app, windowID);
            }
            if (window != NULL) {
                id isMinimized = copyAttribute(window, kAXMinimizedAttribute);
                if ([isMinimized isKindOfClass:NSNumber.class] && [isMinimized boolValue]) {
                    break;
                }
                walk = [AXFillWalk new];
                walk.frames = [NSMutableArray array];
                walk.fields = [NSMutableArray array];
                walk.buttons = [NSMutableArray array];
                walkElement(window, -1, 0, walk);
                walk.isIncomplete = walk.isIncomplete || isMessagingFailed || isOperationCut;
                // Chromium builds its web tree only for an assistive app that asks with AXEnhancedUserInterface,
                // and shows an empty web area until then.
                BOOL isReady = !walk.isIncomplete && walk.frames.count > 0 && (!walk.isTopAreaEmpty || isProcessReady(pid));
                if (isReady) {
                    setProcessReady(pid);
                    break;
                }
            }
            if (isOperationCut || [readyDeadline timeIntervalSinceNow] <= 0) {
                break;
            }
            if (!isEnabled) {
                setBoolean(app, CFSTR("AXManualAccessibility"), YES);
                setBoolean(app, CFSTR("AXEnhancedUserInterface"), YES);
                isEnabled = YES;
            }
            if (!pauseWithin(0.02)) {
                break;
            }
        }
        CFRelease(app);
        BOOL isUnreadable = isOperationCut || (walk != nil && walk.isIncomplete) || (window == NULL && isMessagingFailed);
        if (isUnreadable || window == NULL || walk == nil) {
            if (window != NULL) {
                CFRelease(window);
            }
            // A window the window server lists but the app's accessibility windows do not is on another Space.
            *status = isUnreadable ? axfillUnreadable : axfillNotVisible;
            return NULL;
        }
        uint64_t windowIdentifier = registerElement(window);
        CFRelease(window);
        int state = readWindowState(pid, windowID);
        if (state != axfillOK) {
            *status = state;
            return NULL;
        }
        return copyJSON(@{
            @"window" : @(windowIdentifier),
            @"frames" : walk.frames,
            @"fields" : walk.fields,
            @"buttons" : walk.buttons,
        });
    }
}

static BOOL isGuardHolding(AXFillGuard guard, AXUIElementRef window, AXUIElementRef element) {
    CGWindowID mapped = 0;
    pid_t elementPID = 0;
    if (!copyWindowID(window, &mapped) || mapped != guard.windowID ||
        AXUIElementGetPid(element, &elementPID) != kAXErrorSuccess || elementPID != guard.pid) {
        return NO;
    }
    NSMutableArray *origins = [NSMutableArray array];
    id current = (__bridge id)element;
    BOOL isInWindow = NO;
    for (int depth = 0; depth <= maximumDepth && isElement(current); depth++) {
        AXUIElementRef currentElement = (__bridge AXUIElementRef)current;
        if (CFEqual(currentElement, window)) {
            isInWindow = YES;
            break;
        }
        if ([copyString(currentElement, kAXRoleAttribute) isEqualToString:webAreaRole]) {
            [origins addObject:originKey(copyURL(currentElement))];
        }
        current = copyAttribute(currentElement, kAXParentAttribute);
    }
    if (!isInWindow || ![[origins componentsJoinedByString:@"\n"] isEqualToString:@(guard.origins)]) {
        return NO;
    }
    return !guard.isSecure || [copyString(element, kAXSubroleAttribute) isEqualToString:(__bridge NSString *)kAXSecureTextFieldSubrole];
}

static int checkGuard(AXFillGuard guard, AXUIElementRef *checked) {
    if (readWindowState(guard.pid, guard.windowID) != axfillOK) {
        return axfillChanged;
    }
    AXUIElementRef window = copyRegisteredElement(guard.window);
    AXUIElementRef element = copyRegisteredElement(guard.element);
    BOOL isHolding = window != NULL && element != NULL && isGuardHolding(guard, window, element);
    if (window != NULL) {
        CFRelease(window);
    }
    if (!isHolding) {
        if (element != NULL) {
            CFRelease(element);
        }
        return axfillChanged;
    }
    *checked = element;
    return axfillOK;
}

static void postReturn(int pid) {
    for (int isDown = 1; isDown >= 0; isDown--) {
        CGEventRef event = CGEventCreateKeyboardEvent(NULL, keyReturn, isDown);
        if (event == NULL) {
            continue;
        }
        CGEventSetFlags(event, 0);
        CGEventPostToPid(pid, event);
        CFRelease(event);
    }
}

// isFocused asks the field itself: a browser in the background reports no focused element for the app, but its field
// knows it holds the keyboard focus that keys posted to the process reach.
static BOOL isFocused(AXUIElementRef element) {
    id isFocused = copyAttribute(element, kAXFocusedAttribute);
    return [isFocused isKindOfClass:NSNumber.class] && [isFocused boolValue];
}

static BOOL focus(AXUIElementRef element) {
    setBoolean(element, kAXFocusedAttribute, YES);
    for (int attempt = 0; attempt < 50; attempt++) {
        if (isFocused(element)) {
            return YES;
        }
        if (!pauseWithin(0.02)) {
            return NO;
        }
    }
    return NO;
}

static BOOL waitForCount(AXUIElementRef element, int expected, int attempts) {
    for (int attempt = 0; attempt < attempts; attempt++) {
        if (countCharacters(element) == expected) {
            return YES;
        }
        if (!pauseWithin(0.02)) {
            return NO;
        }
    }
    return NO;
}

int axfillSetValue(AXFillGuard guard, const char *value) {
    @autoreleasepool {
        beginOperation(guard.limit);
        AXUIElementRef element = NULL;
        int result = checkGuard(guard, &element);
        if (result != axfillOK) {
            return endOperation(result);
        }
        NSString *text = [NSString stringWithUTF8String:value] ?: @"";
        result = setAttribute(element, kAXValueAttribute, (__bridge CFStringRef)text) == kAXErrorSuccess ? axfillOK : axfillFailed;
        // Chromium applies the write in the page and updates the count about 20 ms later; a hidden page never does.
        if (result == axfillOK) {
            waitForCount(element, (int)text.length, 15);
        }
        CFRelease(element);
        return endOperation(result);
    }
}

int axfillPress(AXFillGuard guard) {
    @autoreleasepool {
        beginOperation(guard.limit);
        AXUIElementRef element = NULL;
        int result = checkGuard(guard, &element);
        if (result != axfillOK) {
            return endOperation(result);
        }
        result = press(element) == kAXErrorSuccess ? axfillOK : axfillFailed;
        CFRelease(element);
        return endOperation(result);
    }
}

int axfillPressReturn(AXFillGuard guard) {
    @autoreleasepool {
        beginOperation(guard.limit);
        AXUIElementRef element = NULL;
        int result = checkGuard(guard, &element);
        if (result != axfillOK) {
            return endOperation(result);
        }
        if (focus(element)) {
            postReturn(guard.pid);
        } else {
            result = axfillFailed;
        }
        CFRelease(element);
        return endOperation(result);
    }
}

int axfillCountCharacters(AXFillGuard guard, int *count) {
    @autoreleasepool {
        beginOperation(guard.limit);
        AXUIElementRef element = NULL;
        int result = checkGuard(guard, &element);
        if (result != axfillOK) {
            return endOperation(result);
        }
        *count = countCharacters(element);
        CFRelease(element);
        return endOperation(isMessagingFailed ? axfillUnreadable : axfillOK);
    }
}

static NSArray *copyClasses(AXUIElementRef element) {
    id classes = copyAttribute(element, CFSTR("AXDOMClassList"));
    return [classes isKindOfClass:NSArray.class] ? classes : @[];
}

static NSDictionary *describeFrame(AXUIElementRef element) {
    CGRect frame = CGRectZero;
    copyFrame(element, &frame);
    return @{@"x" : @(frame.origin.x), @"y" : @(frame.origin.y), @"width" : @(frame.size.width), @"height" : @(frame.size.height)};
}

static BOOL describePrompt(AXUIElementRef element, NSArray *ancestorClasses, int depth, int *nodes, NSMutableArray *buttonTitles,
                           NSMutableArray *controls, int *secureFieldCount) {
    if (depth > promptDepth || ++*nodes > maximumPromptNodes) {
        return NO;
    }
    NSString *role = copyString(element, kAXRoleAttribute) ?: @"";
    NSArray *classes = copyClasses(element);
    if ([copyString(element, kAXSubroleAttribute) isEqualToString:(__bridge NSString *)kAXSecureTextFieldSubrole]) {
        (*secureFieldCount)++;
    }
    if ([role isEqualToString:(__bridge NSString *)kAXButtonRole]) {
        [buttonTitles addObject:copyButtonTitle(element)];
    }
    if ([@[ (__bridge NSString *)kAXButtonRole, (__bridge NSString *)kAXCheckBoxRole, (__bridge NSString *)kAXPopUpButtonRole ] containsObject:role]) {
        [controls addObject:@{
            @"id" : @(registerElement(element)),
            @"role" : role,
            @"classes" : classes,
            @"ancestorClasses" : ancestorClasses,
            @"frame" : describeFrame(element),
        }];
    }
    NSArray *childAncestorClasses = [classes arrayByAddingObjectsFromArray:ancestorClasses];
    for (id child in copyChildren(element)) {
        if (!describePrompt((__bridge AXUIElementRef)child, childAncestorClasses, depth + 1, nodes, buttonTitles, controls, secureFieldCount)) {
            return NO;
        }
    }
    return YES;
}

static NSArray *listPromptElements(AXUIElementRef app) {
    NSMutableArray *windows = [NSMutableArray array];
    for (id window in [copyAttribute(app, kAXWindowsAttribute) ?: @[] arrayByAddingObjectsFromArray:copyChildren(app)]) {
        BOOL isListed = NO;
        for (id listed in windows) {
            isListed = isListed || CFEqual((__bridge CFTypeRef)listed, (__bridge CFTypeRef)window);
        }
        if (!isListed && [copyString((__bridge AXUIElementRef)window, kAXRoleAttribute) isEqualToString:(__bridge NSString *)kAXWindowRole]) {
            [windows addObject:window];
        }
    }
    NSMutableArray *prompts = [NSMutableArray array];
    for (id window in windows) {
        AXUIElementRef windowElement = (__bridge AXUIElementRef)window;
        if (![copyString(windowElement, kAXSubroleAttribute) isEqualToString:(__bridge NSString *)kAXStandardWindowSubrole]) {
            [prompts addObject:window];
            continue;
        }
        for (id child in copyChildren(windowElement)) {
            if ([copyString((__bridge AXUIElementRef)child, kAXRoleAttribute) isEqualToString:(__bridge NSString *)kAXSheetRole]) {
                [prompts addObject:child];
            }
        }
    }
    return prompts;
}

char *axfillListPrompts(int pid, AXFillLimit limit, int *status) {
    @autoreleasepool {
        beginOperation(limit);
        if (!AXIsProcessTrusted()) {
            *status = axfillNoAccess;
            return NULL;
        }
        AXUIElementRef app = AXUIElementCreateApplication(pid);
        NSMutableArray *prompts = [NSMutableArray array];
        BOOL isComplete = YES;
        for (id prompt in listPromptElements(app)) {
            AXUIElementRef element = (__bridge AXUIElementRef)prompt;
            NSMutableArray *buttonTitles = [NSMutableArray array];
            NSMutableArray *controls = [NSMutableArray array];
            int secureFieldCount = 0, nodes = 0;
            isComplete = describePrompt(element, @[], 0, &nodes, buttonTitles, controls, &secureFieldCount) && isComplete;
            uint64_t identifier = registerElement(element);
            notePromptShown(identifier);
            [prompts addObject:@{
                @"id" : @(identifier),
                @"title" : copyString(element, kAXTitleAttribute) ?: copyString(element, kAXDescriptionAttribute) ?: @"",
                @"buttonTitles" : buttonTitles,
                @"secureFieldCount" : @(secureFieldCount),
                @"frame" : describeFrame(element),
                @"controls" : controls,
            }];
        }
        CFRelease(app);
        // A prompt the browser did not finish describing may be the one that offers to save the password.
        *status = endOperation(isComplete && !isMessagingFailed ? axfillOK : axfillUnreadable);
        return *status == axfillOK ? copyJSON(prompts) : NULL;
    }
}

static BOOL isPromptControl(int pid, AXUIElementRef prompt, AXUIElementRef control, const char *classes, const char *ancestorClasses) {
    pid_t promptPID = 0;
    AXUIElementRef app = AXUIElementCreateApplication(pid);
    BOOL isListed = NO;
    for (id listed in listPromptElements(app)) {
        isListed = isListed || CFEqual((__bridge CFTypeRef)listed, prompt);
    }
    CFRelease(app);
    if (!isListed || AXUIElementGetPid(prompt, &promptPID) != kAXErrorSuccess || promptPID != pid ||
        ![copyString(control, kAXRoleAttribute) isEqualToString:(__bridge NSString *)kAXButtonRole] ||
        ![[copyClasses(control) componentsJoinedByString:@"\n"] isEqualToString:@(classes)]) {
        return NO;
    }
    NSMutableArray *found = [NSMutableArray array];
    id ancestor = copyAttribute(control, kAXParentAttribute);
    for (int depth = 0; depth <= promptDepth && isElement(ancestor); depth++) {
        [found addObjectsFromArray:copyClasses((__bridge AXUIElementRef)ancestor)];
        if (CFEqual((__bridge CFTypeRef)ancestor, prompt)) {
            return [[found componentsJoinedByString:@"\n"] isEqualToString:@(ancestorClasses)];
        }
        ancestor = copyAttribute((__bridge AXUIElementRef)ancestor, kAXParentAttribute);
    }
    return NO;
}

int axfillPressPromptControl(int pid, uint64_t prompt, uint64_t control, const char *classes, const char *ancestorClasses, AXFillLimit limit) {
    @autoreleasepool {
        beginOperation(limit);
        AXUIElementRef promptElement = copyRegisteredElement(prompt);
        AXUIElementRef controlElement = copyRegisteredElement(control);
        int result = axfillChanged;
        if (promptElement != NULL && controlElement != NULL && isPromptControl(pid, promptElement, controlElement, classes, ancestorClasses) &&
            waitUntilPressable(prompt)) {
            notePromptPressed(prompt);
            result = press(controlElement) == kAXErrorSuccess ? axfillOK : axfillFailed;
        }
        if (promptElement != NULL) {
            CFRelease(promptElement);
        }
        if (controlElement != NULL) {
            CFRelease(controlElement);
        }
        return endOperation(result);
    }
}

char *axfillListWindows(int pid) {
    @autoreleasepool {
        NSArray *windows = CFBridgingRelease(CGWindowListCopyWindowInfo(kCGWindowListOptionAll, kCGNullWindowID));
        NSMutableArray *found = [NSMutableArray array];
        for (NSDictionary *window in windows) {
            if ([window[(__bridge NSString *)kCGWindowOwnerPID] intValue] == pid && [window[(__bridge NSString *)kCGWindowLayer] intValue] == 0) {
                [found addObject:@{
                    @"id" : window[(__bridge NSString *)kCGWindowNumber],
                    @"isOnScreen" : @([window[(__bridge NSString *)kCGWindowIsOnscreen] boolValue]),
                }];
            }
        }
        return copyJSON(found);
    }
}
