#import <AppKit/AppKit.h>
#include <stdlib.h>
#include <string.h>
#include "appinfo_darwin.h"

char *copyAppDisplayName(const char *bundlePath) {
    @autoreleasepool {
        NSString *path = [NSString stringWithUTF8String:bundlePath];
        NSString *name = [[NSFileManager defaultManager] displayNameAtPath:path];
        if ([name hasSuffix:@".app"]) {
            name = [name stringByDeletingPathExtension];
        }
        return name.length > 0 ? strdup(name.UTF8String) : NULL;
    }
}

void *copyAppIconPNG(const char *bundlePath, int size, int *length) {
    @autoreleasepool {
        *length = 0;
        NSImage *icon = [[NSWorkspace sharedWorkspace] iconForFile:[NSString stringWithUTF8String:bundlePath]];
        NSRect rect = NSMakeRect(0, 0, size, size);
        CGImageRef image = [icon CGImageForProposedRect:&rect context:nil hints:nil];
        if (image == NULL) {
            return NULL;
        }
        NSBitmapImageRep *representation = [[NSBitmapImageRep alloc] initWithCGImage:image];
        NSData *data = [representation representationUsingType:NSBitmapImageFileTypePNG properties:@{}];
        if (data.length == 0) {
            return NULL;
        }
        void *buffer = malloc(data.length);
        memcpy(buffer, data.bytes, data.length);
        *length = (int)data.length;
        return buffer;
    }
}
