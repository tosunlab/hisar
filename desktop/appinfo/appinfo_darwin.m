//go:build darwin && !ios && cgo

#import <AppKit/AppKit.h>
#import <Security/Security.h>
#include <arpa/inet.h>
#include <libproc.h>
#include <stdlib.h>
#include <string.h>
#include "appinfo_darwin.h"

enum {
    signatureStatusUnsigned = 0,
    signatureStatusInvalid = 1,
    signatureStatusDeveloper = 2,
    signatureStatusApple = 3,
};

extern pid_t responsibility_get_pid_responsible_for_pid(pid_t pid);

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

int copyProcessPath(int pid, char *buffer, int size) {
    return proc_pidpath(pid, buffer, (uint32_t)size) > 0 ? 0 : -1;
}

int getResponsiblePID(int pid) {
    return responsibility_get_pid_responsible_for_pid(pid);
}

char *copyAppDisplayName(const char *bundlePath) {
    @autoreleasepool {
        NSString *path = [NSString stringWithUTF8String:bundlePath];
        NSString *name = [[NSFileManager defaultManager] displayNameAtPath:path];
        if ([name hasSuffix:@".app"]) {
            name = [name stringByDeletingPathExtension];
        }
        // The bundle's own name only counts when it extends the name on disk, as iTerm.app's "iTerm2" does; VS Code's "Code" does not.
        NSDictionary *information = [NSDictionary dictionaryWithContentsOfFile:[path stringByAppendingPathComponent:@"Contents/Info.plist"]];
        id bundleName = information[@"CFBundleDisplayName"] ?: information[@"CFBundleName"];
        if (name.length > 0 && [bundleName isKindOfClass:[NSString class]] && [bundleName length] > name.length && [bundleName length] <= 64 &&
            [bundleName rangeOfString:name options:NSCaseInsensitiveSearch].location != NSNotFound) {
            name = bundleName;
        }
        return name.length > 0 ? strdup(name.UTF8String) : NULL;
    }
}

void *copyAppIconPNG(const char *bundlePath, int size, int *length) {
    @autoreleasepool {
        *length = 0;
        NSImage *icon = [[NSWorkspace sharedWorkspace] iconForFile:[NSString stringWithUTF8String:bundlePath]];
        NSBitmapImageRep *representation = [[NSBitmapImageRep alloc] initWithBitmapDataPlanes:NULL
                                                                                  pixelsWide:size
                                                                                  pixelsHigh:size
                                                                               bitsPerSample:8
                                                                             samplesPerPixel:4
                                                                                    hasAlpha:YES
                                                                                    isPlanar:NO
                                                                              colorSpaceName:NSDeviceRGBColorSpace
                                                                                 bytesPerRow:0
                                                                                bitsPerPixel:0];
        if (icon == nil || representation == nil) {
            return NULL;
        }
        representation.size = NSMakeSize(size, size);
        NSGraphicsContext *context = [NSGraphicsContext graphicsContextWithBitmapImageRep:representation];
        if (context == nil) {
            return NULL;
        }
        [NSGraphicsContext saveGraphicsState];
        [NSGraphicsContext setCurrentContext:context];
        context.imageInterpolation = NSImageInterpolationHigh;
        [icon drawInRect:NSMakeRect(0, 0, size, size) fromRect:NSZeroRect operation:NSCompositingOperationCopy fraction:1.0];
        [context flushGraphics];
        [NSGraphicsContext restoreGraphicsState];
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

RunningAppInfo *copyAllRunningApp(int *count) {
    @autoreleasepool {
        NSArray<NSRunningApplication *> *allApplication = [[NSWorkspace sharedWorkspace] runningApplications];
        *count = 0;
        if (allApplication.count == 0) {
            return NULL;
        }
        RunningAppInfo *allRunningApp = calloc(allApplication.count, sizeof(RunningAppInfo));
        for (NSRunningApplication *application in allApplication) {
            NSString *bundlePath = application.bundleURL.path;
            if (bundlePath.length == 0) {
                continue;
            }
            allRunningApp[*count].pid = application.processIdentifier;
            allRunningApp[*count].isRegular = application.activationPolicy == NSApplicationActivationPolicyRegular;
            allRunningApp[*count].bundlePath = strdup(bundlePath.fileSystemRepresentation);
            (*count)++;
        }
        return allRunningApp;
    }
}

void freeAllRunningApp(RunningAppInfo *allRunningApp, int count) {
    for (int index = 0; index < count; index++) {
        free(allRunningApp[index].bundlePath);
    }
    free(allRunningApp);
}

static SecStaticCodeRef createStaticCode(const char *path) {
    NSURL *url = [NSURL fileURLWithPath:[NSString stringWithUTF8String:path]];
    SecStaticCodeRef code = NULL;
    if (SecStaticCodeCreateWithPath((__bridge CFURLRef)url, kSecCSDefaultFlags, &code) != errSecSuccess) {
        return NULL;
    }
    return code;
}

static char *copyCString(id value) {
    if (![value isKindOfClass:[NSString class]] || [value length] == 0) {
        return NULL;
    }
    return strdup([value UTF8String]);
}

char *copyTeamID(const char *path) {
    @autoreleasepool {
        SecStaticCodeRef code = createStaticCode(path);
        if (code == NULL) {
            return NULL;
        }
        CFDictionaryRef information = NULL;
        OSStatus status = SecCodeCopySigningInformation(code, kSecCSSigningInformation, &information);
        CFRelease(code);
        if (status != errSecSuccess || information == NULL) {
            return NULL;
        }
        NSDictionary *dictionary = CFBridgingRelease(information);
        return copyCString(dictionary[(__bridge NSString *)kSecCodeInfoTeamIdentifier]);
    }
}

// Resources are not hashed: checking every file of a large app would hold up the dialog for seconds.
SignatureInfo checkSignature(const char *path) {
    @autoreleasepool {
        SignatureInfo result = {signatureStatusUnsigned, NULL, NULL};
        SecStaticCodeRef code = createStaticCode(path);
        if (code == NULL) {
            result.status = signatureStatusInvalid;
            return result;
        }
        OSStatus validity = SecStaticCodeCheckValidityWithErrors(code, kSecCSDoNotValidateResources | kSecCSCheckAllArchitectures, NULL, NULL);
        if (validity == errSecCSUnsigned) {
            CFRelease(code);
            return result;
        }
        if (validity != errSecSuccess) {
            CFRelease(code);
            result.status = signatureStatusInvalid;
            return result;
        }
        CFDictionaryRef information = NULL;
        if (SecCodeCopySigningInformation(code, kSecCSSigningInformation, &information) != errSecSuccess || information == NULL) {
            CFRelease(code);
            result.status = signatureStatusInvalid;
            return result;
        }
        NSDictionary *dictionary = CFBridgingRelease(information);
        NSNumber *flags = dictionary[(__bridge NSString *)kSecCodeInfoFlags];
        NSArray *allCertificate = dictionary[(__bridge NSString *)kSecCodeInfoCertificates];
        if ((flags.unsignedIntValue & kSecCodeSignatureAdhoc) != 0 || allCertificate.count == 0) {
            CFRelease(code);
            return result;
        }
        SecRequirementRef appleRequirement = NULL;
        SecRequirementCreateWithString(CFSTR("anchor apple"), kSecCSDefaultFlags, &appleRequirement);
        BOOL isApple = appleRequirement != NULL && SecStaticCodeCheckValidity(code, kSecCSDoNotValidateResources, appleRequirement) == errSecSuccess;
        if (appleRequirement != NULL) {
            CFRelease(appleRequirement);
        }
        CFRelease(code);
        result.status = isApple ? signatureStatusApple : signatureStatusDeveloper;
        result.teamID = copyCString(dictionary[(__bridge NSString *)kSecCodeInfoTeamIdentifier]);
        CFStringRef summary = SecCertificateCopySubjectSummary((__bridge SecCertificateRef)allCertificate[0]);
        if (summary != NULL) {
            result.signerName = copyCString((__bridge_transfer NSString *)summary);
        }
        return result;
    }
}

static int formatSocketPeer(struct in_sockinfo *information, char *buffer, socklen_t size) {
    if (information->insi_vflag & INI_IPV4) {
        return inet_ntop(AF_INET, &information->insi_faddr.ina_46.i46a_addr4, buffer, size) != NULL ? 0 : -1;
    }
    if (information->insi_vflag & INI_IPV6) {
        return inet_ntop(AF_INET6, &information->insi_faddr.ina_6, buffer, size) != NULL ? 0 : -1;
    }
    return -1;
}

char *copySSHRemoteHost(int pid) {
    int bufferSize = proc_pidinfo(pid, PROC_PIDLISTFDS, 0, NULL, 0);
    if (bufferSize <= 0) {
        return NULL;
    }
    struct proc_fdinfo *allDescriptor = malloc(bufferSize);
    bufferSize = proc_pidinfo(pid, PROC_PIDLISTFDS, 0, allDescriptor, bufferSize);
    int descriptorCount = bufferSize > 0 ? bufferSize / (int)sizeof(struct proc_fdinfo) : 0;
    char *host = NULL;
    for (int index = 0; index < descriptorCount && host == NULL; index++) {
        if (allDescriptor[index].proc_fdtype != PROX_FDTYPE_SOCKET) {
            continue;
        }
        struct socket_fdinfo socketInformation;
        if (proc_pidfdinfo(pid, allDescriptor[index].proc_fd, PROC_PIDFDSOCKETINFO, &socketInformation, sizeof(socketInformation)) != sizeof(socketInformation)) {
            continue;
        }
        if (socketInformation.psi.soi_kind != SOCKINFO_TCP) {
            continue;
        }
        char address[INET6_ADDRSTRLEN];
        if (formatSocketPeer(&socketInformation.psi.soi_proto.pri_tcp.tcpsi_ini, address, sizeof(address)) == 0) {
            host = strdup(address);
        }
    }
    free(allDescriptor);
    return host;
}
