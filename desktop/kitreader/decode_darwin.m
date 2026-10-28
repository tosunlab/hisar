//go:build darwin && cgo

#import <Foundation/Foundation.h>
#import <CoreGraphics/CoreGraphics.h>
#import <Vision/Vision.h>
#include <math.h>
#include <string.h>

char *hisarKitCodes(const void *bytes, size_t length) {
    @autoreleasepool {
        CGDataProviderRef provider = CGDataProviderCreateWithData(NULL, bytes, length, NULL);
        if (!provider) return NULL;
        CGPDFDocumentRef document = CGPDFDocumentCreateWithProvider(provider);
        CGDataProviderRelease(provider);
        if (!document) return NULL;
        size_t pages = CGPDFDocumentGetNumberOfPages(document);
        if (CGPDFDocumentIsEncrypted(document) || pages == 0 || pages > 4) {
            CGPDFDocumentRelease(document);
            return NULL;
        }
        NSMutableArray<NSString *> *codes = [NSMutableArray array];
        for (size_t pageIndex = 1; pageIndex <= pages && codes.count < 32; pageIndex++) {
            @autoreleasepool {
                CGPDFPageRef page = CGPDFDocumentGetPage(document, pageIndex);
                CGRect box = CGPDFPageGetBoxRect(page, kCGPDFMediaBox);
                if (!isfinite(box.size.width) || !isfinite(box.size.height) || box.size.width <= 0 || box.size.height <= 0) continue;
                double scale = 3072.0 / fmax(box.size.width, box.size.height);
                size_t width = (size_t)ceil(box.size.width * scale);
                size_t height = (size_t)ceil(box.size.height * scale);
                if (width == 0 || height == 0 || width > 3072 || height > 3072) continue;
                CGColorSpaceRef color = CGColorSpaceCreateDeviceRGB();
                CGContextRef context = CGBitmapContextCreate(NULL, width, height, 8, width * 4, color, (CGBitmapInfo)kCGImageAlphaPremultipliedLast);
                CGColorSpaceRelease(color);
                if (!context) continue;
                CGContextSetRGBFillColor(context, 1, 1, 1, 1);
                CGContextFillRect(context, CGRectMake(0, 0, width, height));
                CGContextConcatCTM(context, CGPDFPageGetDrawingTransform(page, kCGPDFMediaBox, CGRectMake(0, 0, width, height), 0, true));
                CGContextDrawPDFPage(context, page);
                CGImageRef image = CGBitmapContextCreateImage(context);
                CGContextRelease(context);
                if (!image) continue;
                VNDetectBarcodesRequest *request = [[VNDetectBarcodesRequest alloc] init];
                request.symbologies = @[VNBarcodeSymbologyQR];
                VNImageRequestHandler *handler = [[VNImageRequestHandler alloc] initWithCGImage:image options:@{}];
                if ([handler performRequests:@[request] error:nil]) {
                    for (VNBarcodeObservation *observation in request.results) {
                        NSString *payload = observation.payloadStringValue;
                        if (payload && [payload lengthOfBytesUsingEncoding:NSUTF8StringEncoding] <= 8192 && codes.count < 32) [codes addObject:payload];
                    }
                }
                [handler release];
                [request release];
                CGImageRelease(image);
            }
        }
        CGPDFDocumentRelease(document);
        NSData *json = [NSJSONSerialization dataWithJSONObject:codes options:0 error:nil];
        if (!json) return NULL;
        return strndup(json.bytes, json.length);
    }
}
