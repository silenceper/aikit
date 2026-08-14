//go:build windows

package library

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"unsafe"

	"golang.org/x/sys/windows"
)

type windowsVerifiedSkillRoot struct{ directory *os.File }

func (root *windowsVerifiedSkillRoot) Readlink(name string) (string, error) {
	return "", &fs.PathError{Op: "readlink", Path: name, Err: fmt.Errorf("anchored raw symlink reads are unsupported on windows")}
}

func openVerifiedSkillRoot(libraryRoot, id string, beforeComponent func(int) error) (VerifiedSkillRoot, error) {
	if _, err := lexicalLibraryPath(libraryRoot, id); err != nil {
		return nil, err
	}
	absRoot, err := filepath.Abs(libraryRoot)
	if err != nil {
		return nil, err
	}
	root16, err := windows.UTF16PtrFromString(filepath.Clean(absRoot))
	if err != nil {
		return nil, err
	}
	current, err := windows.CreateFile(root16, windows.FILE_GENERIC_READ, windows.FILE_SHARE_READ|windows.FILE_SHARE_WRITE|windows.FILE_SHARE_DELETE, nil, windows.OPEN_EXISTING, windows.FILE_FLAG_BACKUP_SEMANTICS|windows.FILE_FLAG_OPEN_REPARSE_POINT, 0)
	if err != nil {
		return nil, fmt.Errorf("open library root without following reparse points: %w", err)
	}
	if err := verifyWindowsDirectory(current); err != nil {
		_ = windows.CloseHandle(current)
		return nil, err
	}
	for index, segment := range strings.Split(id, "/") {
		if beforeComponent != nil {
			if err := beforeComponent(index); err != nil {
				_ = windows.CloseHandle(current)
				return nil, err
			}
		}
		next, err := openWindowsRelative(current, segment, true)
		_ = windows.CloseHandle(current)
		if err != nil {
			return nil, fmt.Errorf("open skill id component without following reparse points: %w", err)
		}
		current = next
	}
	return &windowsVerifiedSkillRoot{directory: os.NewFile(uintptr(current), id)}, nil
}

func (root *windowsVerifiedSkillRoot) Close() error { return root.directory.Close() }

func (root *windowsVerifiedSkillRoot) Open(name string) (fs.File, error) {
	if !fs.ValidPath(name) {
		return nil, &fs.PathError{Op: "open", Path: name, Err: fs.ErrInvalid}
	}
	if name == "." {
		var duplicate windows.Handle
		process := windows.CurrentProcess()
		if err := windows.DuplicateHandle(process, windows.Handle(root.directory.Fd()), process, &duplicate, 0, false, windows.DUPLICATE_SAME_ACCESS); err != nil {
			return nil, err
		}
		return os.NewFile(uintptr(duplicate), name), nil
	}
	var current windows.Handle
	process := windows.CurrentProcess()
	if err := windows.DuplicateHandle(process, windows.Handle(root.directory.Fd()), process, &current, 0, false, windows.DUPLICATE_SAME_ACCESS); err != nil {
		return nil, err
	}
	segments := strings.Split(name, "/")
	for index, segment := range segments {
		next, err := openWindowsRelative(current, segment, index < len(segments)-1)
		_ = windows.CloseHandle(current)
		if err != nil {
			return nil, &fs.PathError{Op: "open", Path: name, Err: err}
		}
		current = next
	}
	return os.NewFile(uintptr(current), name), nil
}

func openWindowsRelative(parent windows.Handle, segment string, directory bool) (windows.Handle, error) {
	name, err := windows.NewNTUnicodeString(segment)
	if err != nil {
		return windows.InvalidHandle, err
	}
	attributes := &windows.OBJECT_ATTRIBUTES{
		Length:        uint32(unsafe.Sizeof(windows.OBJECT_ATTRIBUTES{})),
		RootDirectory: parent,
		ObjectName:    name,
		Attributes:    windows.OBJ_CASE_INSENSITIVE | windows.OBJ_DONT_REPARSE,
	}
	options := uint32(windows.FILE_OPEN_REPARSE_POINT | windows.FILE_SYNCHRONOUS_IO_NONALERT)
	if directory {
		options |= windows.FILE_DIRECTORY_FILE
	}
	var handle windows.Handle
	var status windows.IO_STATUS_BLOCK
	if err := windows.NtCreateFile(&handle, windows.FILE_GENERIC_READ, attributes, &status, nil, 0, windows.FILE_SHARE_READ|windows.FILE_SHARE_WRITE|windows.FILE_SHARE_DELETE, windows.FILE_OPEN, options, 0, 0); err != nil {
		return windows.InvalidHandle, err
	}
	var info windows.ByHandleFileInformation
	if err := windows.GetFileInformationByHandle(handle, &info); err != nil {
		_ = windows.CloseHandle(handle)
		return windows.InvalidHandle, err
	}
	if info.FileAttributes&windows.FILE_ATTRIBUTE_REPARSE_POINT != 0 || (directory && info.FileAttributes&windows.FILE_ATTRIBUTE_DIRECTORY == 0) {
		_ = windows.CloseHandle(handle)
		return windows.InvalidHandle, fmt.Errorf("path component is a reparse point or wrong type")
	}
	return handle, nil
}

func verifyWindowsDirectory(handle windows.Handle) error {
	var info windows.ByHandleFileInformation
	if err := windows.GetFileInformationByHandle(handle, &info); err != nil {
		return err
	}
	if info.FileAttributes&windows.FILE_ATTRIBUTE_DIRECTORY == 0 || info.FileAttributes&windows.FILE_ATTRIBUTE_REPARSE_POINT != 0 {
		return fmt.Errorf("library root is not a safe directory")
	}
	return nil
}
