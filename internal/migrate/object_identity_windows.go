//go:build windows

package migrate

import (
	"fmt"
	"os"

	"golang.org/x/sys/windows"
)

func objectIdentity(path string, info os.FileInfo) (string, error) {
	if info == nil {
		return "", fmt.Errorf("Windows file identity is unavailable")
	}
	path16, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return "", err
	}
	handle, err := windows.CreateFile(
		path16,
		windows.FILE_READ_ATTRIBUTES,
		windows.FILE_SHARE_READ|windows.FILE_SHARE_WRITE|windows.FILE_SHARE_DELETE,
		nil,
		windows.OPEN_EXISTING,
		windows.FILE_FLAG_BACKUP_SEMANTICS|windows.FILE_FLAG_OPEN_REPARSE_POINT,
		0,
	)
	if err != nil {
		return "", err
	}
	defer windows.CloseHandle(handle)
	var current windows.ByHandleFileInformation
	if err := windows.GetFileInformationByHandle(handle, &current); err != nil {
		return "", err
	}
	return fmt.Sprintf("%d:%d:%d", current.VolumeSerialNumber, current.FileIndexHigh, current.FileIndexLow), nil
}
