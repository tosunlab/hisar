//go:build darwin && cgo

#import <Foundation/Foundation.h>
#include <stdlib.h>
#include <string.h>
#include "version_darwin.h"

char *hisarCopyMainBundleInfoString(const char *key) {
    @autoreleasepool {
        id value = [[NSBundle mainBundle] objectForInfoDictionaryKey:[NSString stringWithUTF8String:key]];
        if (![value isKindOfClass:[NSString class]] || [value length] == 0) {
            return NULL;
        }
        return strdup([value UTF8String]);
    }
}
