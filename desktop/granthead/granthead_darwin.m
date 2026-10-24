//go:build darwin && cgo

#import <Foundation/Foundation.h>
#import <Security/Security.h>
#include <pwd.h>
#include <stdlib.h>
#include <string.h>
#include <unistd.h>
#include "granthead_darwin.h"

// The login keychain by its path: the search list and the default keychain are settings any program the user runs may
// change, and a keychain it made there would answer for Hisar's item, or receive it.
static SecKeychainRef openLoginKeychain(OSStatus *status) {
    struct passwd *user = getpwuid(getuid());
    if (user == NULL || user->pw_dir == NULL) {
        *status = errSecNoSuchKeychain;
        return NULL;
    }
    NSString *path = [[NSString stringWithUTF8String:user->pw_dir] stringByAppendingPathComponent:@"Library/Keychains/login.keychain-db"];
    SecKeychainRef keychain = NULL;
    *status = SecKeychainOpen(path.fileSystemRepresentation, &keychain);
    return keychain;
}

static NSMutableDictionary *createQuery(const char *service, SecKeychainRef keychain) {
    return [@{
        (__bridge id)kSecClass: (__bridge id)kSecClassGenericPassword,
        (__bridge id)kSecAttrService: [NSString stringWithUTF8String:service],
        (__bridge id)kSecAttrAccount: @"head",
        (__bridge id)kSecMatchSearchList: @[(__bridge id)keychain],
    } mutableCopy];
}

// hisarGrantHeadSave replaces the item rather than updating it, so the head always goes into an item whose access list
// trusts only this program, as a new item's does; an item another program made or rewrote may let that program read it.
int hisarGrantHeadSave(const char *service, const void *data, int length) {
    OSStatus status;
    SecKeychainRef keychain = openLoginKeychain(&status);
    if (keychain == NULL) {
        return (int)status;
    }
    NSMutableDictionary *query = createQuery(service, keychain);
    status = SecItemDelete((__bridge CFDictionaryRef)query);
    if (status == errSecSuccess || status == errSecItemNotFound) {
        [query removeObjectForKey:(__bridge id)kSecMatchSearchList];
        query[(__bridge id)kSecUseKeychain] = (__bridge id)keychain;
        query[(__bridge id)kSecAttrLabel] = @"Hisar agent grants";
        query[(__bridge id)kSecValueData] = [NSData dataWithBytes:data length:length];
        status = SecItemAdd((__bridge CFDictionaryRef)query, NULL);
    }
    CFRelease(keychain);
    return (int)status;
}

int hisarGrantHeadLoad(const char *service, void **data, int *length) {
    OSStatus status;
    SecKeychainRef keychain = openLoginKeychain(&status);
    if (keychain == NULL) {
        return (int)status;
    }
    NSMutableDictionary *query = createQuery(service, keychain);
    query[(__bridge id)kSecReturnData] = @YES;
    query[(__bridge id)kSecMatchLimit] = (__bridge id)kSecMatchLimitOne;
    CFTypeRef result = NULL;
    status = SecItemCopyMatching((__bridge CFDictionaryRef)query, &result);
    CFRelease(keychain);
    if (status != errSecSuccess) {
        return (int)status;
    }
    NSData *value = (__bridge_transfer NSData *)result;
    *length = (int)value.length;
    *data = malloc(value.length);
    memcpy(*data, value.bytes, value.length);
    return 0;
}

int hisarGrantHeadDelete(const char *service) {
    OSStatus status;
    SecKeychainRef keychain = openLoginKeychain(&status);
    if (keychain == NULL) {
        return (int)status;
    }
    status = SecItemDelete((__bridge CFDictionaryRef)createQuery(service, keychain));
    CFRelease(keychain);
    return status == errSecItemNotFound ? 0 : (int)status;
}

char *hisarGrantHeadDescribe(int status) {
    NSString *message = (__bridge_transfer NSString *)SecCopyErrorMessageString((OSStatus)status, NULL);
    return strdup(message != nil ? message.UTF8String : "unknown error");
}
