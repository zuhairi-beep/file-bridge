//go:build windows

package main

import (
	"fmt"
	"syscall"
	"unsafe"
)

var mutexName = "Local\\FileBridge_SingleInstance_v1"

func platformEnsureSingleInstance() (func(), error) {
	kernel32 := syscall.NewLazyDLL("kernel32.dll")
	createMutex := kernel32.NewProc("CreateMutexW")
	closeHandle := kernel32.NewProc("CloseHandle")
	name, _ := syscall.UTF16PtrFromString(mutexName)
	r0, _, err := createMutex.Call(0, 0, uintptr(unsafe.Pointer(name)))
	if r0 == 0 {
		return nil, fmt.Errorf("CreateMutex: %v", err)
	}
	handle := syscall.Handle(r0)
	const errorAlreadyExists = 183
	if err != nil && err.(syscall.Errno) == syscall.Errno(errorAlreadyExists) {
		closeHandle.Call(uintptr(handle))
		tryFocusExistingWindow()
		return nil, ErrAlreadyRunning
	}
	return func() { closeHandle.Call(uintptr(handle)) }, nil
}

func tryFocusExistingWindow() {
	user32 := syscall.NewLazyDLL("user32.dll")
	findWindow := user32.NewProc("FindWindowW")
	showWindow := user32.NewProc("ShowWindow")
	setForeground := user32.NewProc("SetForegroundWindow")
	title, _ := syscall.UTF16PtrFromString("File Bridge")
	hwnd, _, _ := findWindow.Call(0, uintptr(unsafe.Pointer(title)))
	if hwnd == 0 {
		return
	}
	const swRestore = 9
	showWindow.Call(hwnd, swRestore)
	setForeground.Call(hwnd)
}
