//go:build windows

package library

import (
	"fmt"
	"os"
	"syscall"

	"golang.org/x/sys/windows"
)

func fileIdentity(info os.FileInfo) (string, error) {
	data, ok := info.Sys().(*syscall.Win32FileAttributeData)
	if !ok {
		return "", fmt.Errorf("Windows file identity is unavailable")
	}
	return fmt.Sprintf("%d:%d:%d:%d", data.CreationTime.HighDateTime, data.CreationTime.LowDateTime, data.FileSizeHigh, data.FileSizeLow), nil
}

func pathIdentity(path string) (string, error) {
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
	return fmt.Sprintf("%d:%d:%d", info.VolumeSerialNumber, info.FileIndexHigh, info.FileIndexLow), nil
}
