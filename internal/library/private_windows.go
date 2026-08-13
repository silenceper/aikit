//go:build windows

package library

import (
	"fmt"
	"os"
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"
)

// Windows does not expose POSIX owner/group/other mode bits. Directories
// created here inherit the current user's ACL; reject reparse points instead
// of applying an os.Chmod check that can never establish mode 0700.
func hardenPrivateDirectory(path string) error {
	info, err := os.Lstat(path)
	if err != nil {
		return fmt.Errorf("inspect cache directory: %w", err)
	}
	attributes, ok := info.Sys().(*syscall.Win32FileAttributeData)
	if !ok {
		return fmt.Errorf("inspect cache directory %q: unavailable Windows file attributes", path)
	}
	if !windowsDirectoryAttributesSafe(attributes.FileAttributes) {
		return fmt.Errorf("cache path %q is not a safe directory", path)
	}
	user, err := windows.GetCurrentProcessToken().GetTokenUser()
	if err != nil {
		return fmt.Errorf("get current user for private cache ACL: %w", err)
	}
	system, err := windows.CreateWellKnownSid(windows.WinLocalSystemSid)
	if err != nil {
		return fmt.Errorf("get SYSTEM SID for private cache ACL: %w", err)
	}
	administrators, err := windows.CreateWellKnownSid(windows.WinBuiltinAdministratorsSid)
	if err != nil {
		return fmt.Errorf("get Administrators SID for private cache ACL: %w", err)
	}
	entries := []windows.EXPLICIT_ACCESS{
		privateDirectoryAccess(user.User.Sid, windows.TRUSTEE_IS_USER),
		privateDirectoryAccess(system, windows.TRUSTEE_IS_USER),
		privateDirectoryAccess(administrators, windows.TRUSTEE_IS_GROUP),
	}
	acl, err := windows.ACLFromEntries(entries, nil)
	if err != nil {
		return fmt.Errorf("build private cache ACL: %w", err)
	}
	if err := windows.SetNamedSecurityInfo(path, windows.SE_FILE_OBJECT, windows.DACL_SECURITY_INFORMATION|windows.PROTECTED_DACL_SECURITY_INFORMATION, nil, nil, acl, nil); err != nil {
		return fmt.Errorf("set private cache ACL: %w", err)
	}
	descriptor, err := windows.GetNamedSecurityInfo(path, windows.SE_FILE_OBJECT, windows.DACL_SECURITY_INFORMATION)
	if err != nil {
		return fmt.Errorf("verify private cache ACL: %w", err)
	}
	control, _, err := descriptor.Control()
	if err != nil || control&windows.SE_DACL_PROTECTED == 0 {
		return fmt.Errorf("cache directory %q DACL is not protected", path)
	}
	dacl, _, err := descriptor.DACL()
	if err != nil || dacl == nil {
		return fmt.Errorf("cache directory %q has no private DACL", path)
	}
	allowed := []*windows.SID{user.User.Sid, system, administrators}
	if dacl.AceCount != uint16(len(allowed)) {
		return fmt.Errorf("cache directory %q DACL contains unexpected entries", path)
	}
	for index := uint32(0); index < uint32(dacl.AceCount); index++ {
		var ace *windows.ACCESS_ALLOWED_ACE
		if err := windows.GetAce(dacl, index, &ace); err != nil {
			return fmt.Errorf("verify private cache ACL entry: %w", err)
		}
		if ace.Header.AceType != windows.ACCESS_ALLOWED_ACE_TYPE || ace.Mask&windows.GENERIC_ALL == 0 {
			return fmt.Errorf("cache directory %q DACL contains an unsafe entry", path)
		}
		sid := (*windows.SID)(unsafe.Pointer(&ace.SidStart))
		matched := false
		for _, expected := range allowed {
			if sid.Equals(expected) {
				matched = true
				break
			}
		}
		if !matched {
			return fmt.Errorf("cache directory %q DACL grants an unexpected principal", path)
		}
	}
	return nil
}

func privateDirectoryAccess(sid *windows.SID, trusteeType windows.TRUSTEE_TYPE) windows.EXPLICIT_ACCESS {
	return windows.EXPLICIT_ACCESS{
		AccessPermissions: windows.ACCESS_MASK(windows.GENERIC_ALL),
		AccessMode:        windows.GRANT_ACCESS,
		Inheritance:       windows.SUB_CONTAINERS_AND_OBJECTS_INHERIT,
		Trustee: windows.TRUSTEE{
			TrusteeForm:  windows.TRUSTEE_IS_SID,
			TrusteeType:  trusteeType,
			TrusteeValue: windows.TrusteeValueFromSID(sid),
		},
	}
}

func windowsDirectoryAttributesSafe(attributes uint32) bool {
	return attributes&windows.FILE_ATTRIBUTE_DIRECTORY != 0 && attributes&windows.FILE_ATTRIBUTE_REPARSE_POINT == 0
}
