//go:build windows

package library

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"unsafe"

	"golang.org/x/sys/windows"
)

func secureReadRegular(root, relative string, expected os.FileInfo) ([]byte, error) {
	root16, err := windows.UTF16PtrFromString(root)
	if err != nil {
		return nil, err
	}
	rootHandle, err := windows.CreateFile(root16, windows.FILE_GENERIC_READ, windows.FILE_SHARE_READ|windows.FILE_SHARE_WRITE|windows.FILE_SHARE_DELETE, nil, windows.OPEN_EXISTING, windows.FILE_FLAG_BACKUP_SEMANTICS|windows.FILE_FLAG_OPEN_REPARSE_POINT, 0)
	if err != nil {
		return nil, fmt.Errorf("open source root without following reparse points: %w", err)
	}
	defer windows.CloseHandle(rootHandle)
	var rootInfo windows.ByHandleFileInformation
	if err := windows.GetFileInformationByHandle(rootHandle, &rootInfo); err != nil {
		return nil, fmt.Errorf("inspect source root: %w", err)
	}
	if rootInfo.FileAttributes&windows.FILE_ATTRIBUTE_DIRECTORY == 0 || rootInfo.FileAttributes&windows.FILE_ATTRIBUTE_REPARSE_POINT != 0 {
		return nil, fmt.Errorf("source root is not a safe directory")
	}
	current := rootHandle
	owned := windows.InvalidHandle
	defer func() {
		if owned != windows.InvalidHandle {
			_ = windows.CloseHandle(owned)
		}
	}()
	segments := strings.Split(filepath.ToSlash(relative), "/")
	for index, segment := range segments {
		if segment == "" || segment == "." || segment == ".." {
			return nil, fmt.Errorf("unsafe source path %q", relative)
		}
		name, err := windows.NewNTUnicodeString(segment)
		if err != nil {
			return nil, err
		}
		attributes := &windows.OBJECT_ATTRIBUTES{
			Length:        uint32(unsafe.Sizeof(windows.OBJECT_ATTRIBUTES{})),
			RootDirectory: current,
			ObjectName:    name,
			Attributes:    windows.OBJ_CASE_INSENSITIVE | windows.OBJ_DONT_REPARSE,
		}
		var handle windows.Handle
		var status windows.IO_STATUS_BLOCK
		options := uint32(windows.FILE_OPEN_REPARSE_POINT | windows.FILE_SYNCHRONOUS_IO_NONALERT)
		if index < len(segments)-1 {
			options |= windows.FILE_DIRECTORY_FILE
		} else {
			options |= windows.FILE_NON_DIRECTORY_FILE
		}
		err = windows.NtCreateFile(&handle, windows.FILE_GENERIC_READ, attributes, &status, nil, 0, windows.FILE_SHARE_READ|windows.FILE_SHARE_WRITE|windows.FILE_SHARE_DELETE, windows.FILE_OPEN, options, 0, 0)
		if err != nil {
			return nil, fmt.Errorf("open source path component without following reparse points: %w", err)
		}
		if owned != windows.InvalidHandle {
			_ = windows.CloseHandle(owned)
		}
		owned = handle
		current = handle
	}
	handle := owned
	owned = windows.InvalidHandle
	path := filepath.Join(root, relative)
	file := os.NewFile(uintptr(handle), path)
	if file == nil {
		_ = windows.CloseHandle(handle)
		return nil, fmt.Errorf("open source file")
	}
	defer file.Close()
	opened, err := file.Stat()
	if err != nil || !opened.Mode().IsRegular() || opened.Mode()&os.ModeSymlink != 0 {
		return nil, fmt.Errorf("source file changed type or became a reparse point")
	}
	if err := sameSourceFile(expected, opened); err != nil {
		return nil, err
	}
	content, err := io.ReadAll(file)
	if err != nil {
		return nil, err
	}
	after, err := file.Stat()
	if err != nil {
		return nil, err
	}
	if err := sameSourceFile(opened, after); err != nil {
		return nil, fmt.Errorf("source changed while reading: %w", err)
	}
	return content, nil
}

func sameSourceFile(expected, actual os.FileInfo) error {
	want, err := fileIdentity(expected)
	if err != nil {
		return err
	}
	got, err := fileIdentity(actual)
	if err != nil {
		return err
	}
	if want != got || expected.Size() != actual.Size() || !expected.ModTime().Equal(actual.ModTime()) {
		return fmt.Errorf("source file identity changed")
	}
	return nil
}
