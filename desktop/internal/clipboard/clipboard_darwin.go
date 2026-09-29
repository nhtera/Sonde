// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

//go:build darwin && cgo

package clipboard

/*
#cgo CFLAGS: -x objective-c -fobjc-arc
#cgo LDFLAGS: -framework AppKit
#import <AppKit/AppKit.h>

// setConcealed writes s with the concealed and transient markers that
// clipboard managers honor, and returns the pasteboard's change count.
static long setConcealed(const char *s) {
	@autoreleasepool {
		NSPasteboard *pb = [NSPasteboard generalPasteboard];
		[pb clearContents];
		[pb setString:[NSString stringWithUTF8String:s] forType:NSPasteboardTypeString];
		[pb setString:@"" forType:@"org.nspasteboard.ConcealedType"];
		[pb setString:@"" forType:@"org.nspasteboard.TransientType"];
		return (long)[pb changeCount];
	}
}

// clearIfCount clears the pasteboard when nothing was copied since count.
static void clearIfCount(long count) {
	@autoreleasepool {
		NSPasteboard *pb = [NSPasteboard generalPasteboard];
		if ((long)[pb changeCount] == count) {
			[pb clearContents];
		}
	}
}
*/
import "C"

import "unsafe"

func writeConcealed(text string) (any, error) {
	cs := C.CString(text)
	defer C.free(unsafe.Pointer(cs))
	return int64(C.setConcealed(cs)), nil
}

func clearIf(token any) {
	C.clearIfCount(C.long(token.(int64)))
}
