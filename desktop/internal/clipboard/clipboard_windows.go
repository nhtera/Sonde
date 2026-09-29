// Copyright 2026 The Sonde Authors
// SPDX-License-Identifier: Apache-2.0

package clipboard

import (
	"errors"
	"syscall"
	"unsafe"
)

var (
	user32                  = syscall.NewLazyDLL("user32.dll")
	kernel32                = syscall.NewLazyDLL("kernel32.dll")
	openClipboard           = user32.NewProc("OpenClipboard")
	closeClipboard          = user32.NewProc("CloseClipboard")
	emptyClipboard          = user32.NewProc("EmptyClipboard")
	setClipboardData        = user32.NewProc("SetClipboardData")
	registerClipboardFormat = user32.NewProc("RegisterClipboardFormatW")
	getSequenceNumber       = user32.NewProc("GetClipboardSequenceNumber")
	globalAlloc             = kernel32.NewProc("GlobalAlloc")
	globalLock              = kernel32.NewProc("GlobalLock")
	globalUnlock            = kernel32.NewProc("GlobalUnlock")
)

const (
	cfUnicodeText = 13
	gmemMoveable  = 0x0002
)

// writeConcealed sets the text and the formats that keep it out of
// clipboard monitors, the clipboard history and cloud clipboard.
func writeConcealed(text string) (any, error) {
	utf16, err := syscall.UTF16FromString(text)
	if err != nil {
		return nil, err
	}
	if r, _, _ := openClipboard.Call(0); r == 0 {
		return nil, errors.New("clipboard: busy")
	}
	defer func() { _, _, _ = closeClipboard.Call() }()
	_, _, _ = emptyClipboard.Call() // SetClipboardData reports failures
	if err := setData(cfUnicodeText, unsafe.Slice((*byte)(unsafe.Pointer(&utf16[0])), len(utf16)*2)); err != nil {
		return nil, err
	}
	zero := []byte{0, 0, 0, 0} // a DWORD 0
	for _, name := range []string{"ExcludeClipboardContentFromMonitorProcessing", "CanIncludeInClipboardHistory", "CanUploadToCloudClipboard"} {
		p, _ := syscall.UTF16PtrFromString(name)
		f, _, _ := registerClipboardFormat.Call(uintptr(unsafe.Pointer(p)))
		if f != 0 {
			_ = setData(f, zero)
		}
	}
	seq, _, _ := getSequenceNumber.Call()
	return seq, nil
}

func setData(format uintptr, data []byte) error {
	h, _, _ := globalAlloc.Call(gmemMoveable, uintptr(len(data)))
	if h == 0 {
		return errors.New("clipboard: out of memory")
	}
	p, _, _ := globalLock.Call(h)
	if p == 0 {
		return errors.New("clipboard: lock failed")
	}
	// p is a locked global memory block, not Go memory.
	copy(unsafe.Slice((*byte)(unsafe.Add(nil, p)), len(data)), data)
	_, _, _ = globalUnlock.Call(h) // unlocking cannot fail usefully
	if r, _, _ := setClipboardData.Call(format, h); r == 0 {
		return errors.New("clipboard: set failed")
	}
	return nil
}

func clearIf(token any) {
	seq, _, _ := getSequenceNumber.Call()
	if seq != token.(uintptr) {
		return
	}
	if r, _, _ := openClipboard.Call(0); r == 0 {
		return
	}
	_, _, _ = emptyClipboard.Call() // best effort
	_, _, _ = closeClipboard.Call()
}
