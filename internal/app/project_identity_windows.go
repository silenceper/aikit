//go:build windows

package app

import (
	"crypto/sha256"
	"fmt"
	"os"
	"syscall"

	"golang.org/x/sys/windows"
)

func projectPathIdentity(path string) (string, error) {
	path16, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return "", err
	}
	handle, err := windows.CreateFile(path16, windows.FILE_READ_ATTRIBUTES, windows.FILE_SHARE_READ|windows.FILE_SHARE_WRITE|windows.FILE_SHARE_DELETE, nil, windows.OPEN_EXISTING, windows.FILE_FLAG_BACKUP_SEMANTICS|windows.FILE_FLAG_OPEN_REPARSE_POINT, 0)
	if err != nil {
		return "", err
	}
	defer windows.CloseHandle(handle)
	var info windows.ByHandleFileInformation
	if err := windows.GetFileInformationByHandle(handle, &info); err != nil {
		return "", err
	}
	if info.FileAttributes&windows.FILE_ATTRIBUTE_DIRECTORY == 0 || info.FileAttributes&windows.FILE_ATTRIBUTE_REPARSE_POINT != 0 {
		return "", fmt.Errorf("project path is not a safe directory")
	}
	payload := fmt.Sprintf("aikit-project-v1\x00%s\x00%d:%d:%d", path, info.VolumeSerialNumber, info.FileIndexHigh, info.FileIndexLow)
	return fmt.Sprintf("v1:%x", sha256.Sum256([]byte(payload))), nil
}

func projectPathComponentUnsafe(info os.FileInfo) bool {
	data, ok := info.Sys().(*syscall.Win32FileAttributeData)
	return !ok || data.FileAttributes&windows.FILE_ATTRIBUTE_REPARSE_POINT != 0
}
