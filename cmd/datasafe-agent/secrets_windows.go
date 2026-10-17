//go:build windows

package main

import (
	"errors"
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"
)

// CRED_ENUMERATE_ALL_CREDENTIALS returns TargetName as
// "LegacyGeneric:target=<name>". CredDeleteW rejects that form, and the
// suffix check rejects ':' and '='. Enumerate with filter "*" and flags 0
// so TargetName is the bare generic name the keyring crate wrote
// ("<profile>/<kind>.datasafe-desktop"). The prefixed form is still
// accepted: the prefix is stripped and CredDeleteW gets the bare name.
const credTypeGeneric = 1

var (
	errPurgeSecrets = errors.New("purge-secrets")
	errPurgeFlags   = errors.New("purge-flags")

	modAdvapi32       = windows.NewLazySystemDLL("advapi32.dll")
	procCredEnumerate = modAdvapi32.NewProc("CredEnumerateW")
	procCredDelete    = modAdvapi32.NewProc("CredDeleteW")
	procCredFree      = modAdvapi32.NewProc("CredFree")
)

// credentialW matches CREDENTIALW. Only Type and TargetName are read.
// CredentialBlob is never dereferenced.
type credentialW struct {
	Flags              uint32
	Type               uint32
	TargetName         *uint16
	Comment            *uint16
	LastWritten        windows.Filetime
	CredentialBlobSize uint32
	CredentialBlob     uintptr
	Persist            uint32
	AttributeCount     uint32
	Attributes         uintptr
	TargetAlias        *uint16
	UserName           *uint16
}

func purgeDesktopSecrets() (int, error) {
	removed, err := deleteDesktopCredentials("*", 0)
	if errors.Is(err, errPurgeFlags) {
		removed, err = deleteDesktopCredentials("", 0)
	}
	if err != nil {
		return removed, err
	}
	left, err := desktopCredentialNames("*", 0)
	if errors.Is(err, errPurgeFlags) {
		left, err = desktopCredentialNames("", 0)
	}
	if err != nil {
		return removed, err
	}
	if len(left) > 0 {
		return removed, errPurgeSecrets
	}
	return removed, nil
}

func deleteDesktopCredentials(filter string, flags uintptr) (int, error) {
	buf, count, err := credEnumerate(filter, flags)
	if err != nil || buf == 0 || count == 0 {
		return 0, err
	}
	defer procCredFree.Call(buf)
	ptrs := unsafe.Slice((*uintptr)(unsafe.Pointer(buf)), count)
	removed := 0
	seen := map[string]struct{}{}
	for _, p := range ptrs {
		bare, credType, ok := desktopCredentialAt(p)
		if !ok {
			continue
		}
		if _, dup := seen[bare]; dup {
			continue
		}
		seen[bare] = struct{}{}
		if err := credDelete(bare, credType); err != nil {
			return removed, err
		}
		removed++
	}
	return removed, nil
}

func desktopCredentialNames(filter string, flags uintptr) ([]string, error) {
	buf, count, err := credEnumerate(filter, flags)
	if err != nil || buf == 0 || count == 0 {
		return nil, err
	}
	defer procCredFree.Call(buf)
	ptrs := unsafe.Slice((*uintptr)(unsafe.Pointer(buf)), count)
	var names []string
	for _, p := range ptrs {
		bare, _, ok := desktopCredentialAt(p)
		if ok {
			names = append(names, bare)
		}
	}
	return names, nil
}

func desktopCredentialAt(p uintptr) (string, uint32, bool) {
	if p == 0 {
		return "", 0, false
	}
	cred := (*credentialW)(unsafe.Pointer(p))
	if cred.TargetName == nil {
		return "", 0, false
	}
	raw := windows.UTF16PtrToString(cred.TargetName)
	bare := bareCredentialTarget(raw)
	if !isDesktopCredential(bare) {
		return "", 0, false
	}
	credType := cred.Type
	if credType == 0 || raw != bare {
		credType = credTypeGeneric
	}
	return bare, credType, true
}

func credEnumerate(filter string, flags uintptr) (uintptr, uint32, error) {
	var filterPtr uintptr
	if filter != "" {
		name, err := windows.UTF16PtrFromString(filter)
		if err != nil {
			return 0, 0, errPurgeSecrets
		}
		filterPtr = uintptr(unsafe.Pointer(name))
	}
	var count uint32
	var buf uintptr
	r, _, callErr := procCredEnumerate.Call(filterPtr, flags, uintptr(unsafe.Pointer(&count)), uintptr(unsafe.Pointer(&buf)))
	if r == 0 {
		errno, _ := callErr.(syscall.Errno)
		if errno == windows.ERROR_NOT_FOUND {
			return 0, 0, nil
		}
		if errno == windows.ERROR_INVALID_FLAGS || errno == windows.ERROR_INVALID_PARAMETER {
			return 0, 0, errPurgeFlags
		}
		return 0, 0, errPurgeSecrets
	}
	return buf, count, nil
}

func credDelete(target string, credType uint32) error {
	name, err := windows.UTF16PtrFromString(target)
	if err != nil {
		return errPurgeSecrets
	}
	r, _, callErr := procCredDelete.Call(uintptr(unsafe.Pointer(name)), uintptr(credType), 0)
	if r == 0 {
		errno, _ := callErr.(syscall.Errno)
		if errno == windows.ERROR_NOT_FOUND {
			return nil
		}
		return errPurgeSecrets
	}
	return nil
}
