#import <AppKit/AppKit.h>
#include <libproc.h>
#include <stdlib.h>
#include <string.h>
#include "appinfo_darwin.h"

int copyProcessWorkingDirectory(int pid, char *buffer, int size) {
    struct proc_vnodepathinfo information;
    if (proc_pidinfo(pid, PROC_PIDVNODEPATHINFO, 0, &information, sizeof(information)) != sizeof(information)) {
        return -1;
    }
    if (information.pvi_cdir.vip_path[0] == '\0') {
        return -1;
    }
    strlcpy(buffer, information.pvi_cdir.vip_path, size);
    return 0;
}

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
