//go:build windows

package library

import (
	"os"
	"syscall"
	"testing"

	"golang.org/x/sys/windows"
)

func TestWindowsDirectoryAttributesRejectReparsePoints(t *testing.T) {
	if !windowsDirectoryAttributesSafe(windows.FILE_ATTRIBUTE_DIRECTORY) {
		t.Fatal("ordinary directory attributes were rejected")
	}
	if windowsDirectoryAttributesSafe(windows.FILE_ATTRIBUTE_DIRECTORY | windows.FILE_ATTRIBUTE_REPARSE_POINT) {
		t.Fatal("directory reparse point was accepted")
	}
}

func TestEnsurePrivateDirectoryAcceptsNormalWindowsDirectory(t *testing.T) {
	directory := t.TempDir()
	if err := ensurePrivateDirectory(directory); err != nil {
		t.Fatal(err)
	}
	descriptor, err := windows.GetNamedSecurityInfo(directory, windows.SE_FILE_OBJECT, windows.DACL_SECURITY_INFORMATION|windows.PROTECTED_DACL_SECURITY_INFORMATION)
	if err != nil {
		t.Fatal(err)
	}
	dacl, _, err := descriptor.DACL()
	if err != nil || dacl == nil {
		t.Fatalf("private directory has no protected DACL: dacl=%v err=%v", dacl, err)
	}
	control, _, err := descriptor.Control()
	if err != nil || control&windows.SE_DACL_PROTECTED == 0 {
		t.Fatalf("private directory DACL is not protected: control=%#x err=%v", control, err)
	}
	info, err := os.Stat(directory)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := info.Sys().(*syscall.Win32FileAttributeData); !ok {
		t.Fatal("private directory lacks Windows attributes")
	}
}
