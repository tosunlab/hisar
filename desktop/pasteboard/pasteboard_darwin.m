//go:build darwin && !ios && cgo

#import <AppKit/AppKit.h>
#include <stdlib.h>
#include <string.h>
#include "pasteboard_darwin.h"

static NSPasteboardType const concealedType = @"org.nspasteboard.ConcealedType";
static NSPasteboardType const transientType = @"org.nspasteboard.TransientType";

void *hisarPasteboardOpenGeneral(void) {
    @autoreleasepool {
        return (__bridge_retained void *)[NSPasteboard generalPasteboard];
    }
}

void *hisarPasteboardOpenNamed(const char *name) {
    @autoreleasepool {
        return (__bridge_retained void *)[NSPasteboard pasteboardWithName:[NSString stringWithUTF8String:name]];
    }
}

void hisarPasteboardReleaseNamed(void *pasteboardReference) {
    @autoreleasepool {
        NSPasteboard *pasteboard = (__bridge_transfer NSPasteboard *)pasteboardReference;
        [pasteboard releaseGlobally];
    }
}

// Returns the change count clearContents gave Hisar, or -1 when nothing was written. Reading the count after the write
// instead could take a copy another app made in between for Hisar's.
long hisarPasteboardWrite(void *pasteboardReference, const char *text, size_t length) {
    @autoreleasepool {
        NSPasteboard *pasteboard = (__bridge NSPasteboard *)pasteboardReference;
        if (length == 0) {
            return [pasteboard clearContents];
        }
        NSString *string = [[NSString alloc] initWithBytes:text length:length encoding:NSUTF8StringEncoding];
        if (string == nil) {
            return -1;
        }
        NSPasteboardItem *item = [[NSPasteboardItem alloc] init];
        [item setString:string forType:NSPasteboardTypeString];
        [item setData:[NSData data] forType:concealedType];
        [item setData:[NSData data] forType:transientType];
        long changeCount = [pasteboard clearContents];
        if (![pasteboard writeObjects:@[item]]) {
            return -1;
        }
        return changeCount;
    }
}

int hisarPasteboardReadOwnedString(void *pasteboardReference, long ownedChangeCount, char **text, size_t *length) {
    @autoreleasepool {
        NSPasteboard *pasteboard = (__bridge NSPasteboard *)pasteboardReference;
        if (pasteboard.changeCount != ownedChangeCount) {
            return 0;
        }
        NSData *data = [[pasteboard stringForType:NSPasteboardTypeString] dataUsingEncoding:NSUTF8StringEncoding];
        // Another app may have copied while the string was being read.
        if (pasteboard.changeCount != ownedChangeCount) {
            return 0;
        }
        *text = NULL;
        *length = data.length;
        if (data.length > 0) {
            *text = malloc(data.length);
            if (*text == NULL) {
                return 0;
            }
            memcpy(*text, data.bytes, data.length);
        }
        return 1;
    }
}

int hisarPasteboardHasType(void *pasteboardReference, const char *type) {
    @autoreleasepool {
        NSPasteboard *pasteboard = (__bridge NSPasteboard *)pasteboardReference;
        return [pasteboard.types containsObject:[NSString stringWithUTF8String:type]] ? 1 : 0;
    }
}
