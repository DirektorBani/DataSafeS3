//go:build windows

package main

import (
	"errors"
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"
)

// CredEnumerateW's filter is a name prefix plus a trailing asterisk. Our
// targets share a suffix, not a prefix, so the filter "*.datasafe-desktop"
// would not match them. CRED_ENUMERATE_ALL_CREDENTIALS with a nil filter
// returns every target name and omits credential blobs. The suffix check
// then decides what CredDeleteW removes.
const credEnumerateAll = 0x1

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
	n, err := purgeDesktopSecretsFlags(credEnumerateAll)
	if errors.Is(err, errPurgeFlags) {
		return purgeDesktopSecretsFlags(0)
	}
	return n, err
}

func purgeDesktopSecretsFlags(flags uintptr) (int, error) {
	buf, count, err := credEnumerate(flags)
	if err != nil || buf == 0 || count == 0 {
		return 0, err
	}
	defer procCredFree.Call(buf)
	ptrs := unsafe.Slice((*uintptr)(unsafe.Pointer(buf)), count)
	removed := 0
	for _, p := range ptrs {
		if p == 0 {
			continue
		}
		cred := (*credentialW)(unsafe.Pointer(p))
		if cred.TargetName == nil {
			continue
		}
		name := windows.UTF16PtrToString(cred.TargetName)
		if !isDesktopCredential(name) {
			continue
		}
		if err := credDelete(name, cred.Type); err != nil {
			return removed, err
		}
		removed++
	}
	return removed, nil
}

func credEnumerate(flags uintptr) (uintptr, uint32, error) {
	var count uint32
	var buf uintptr
	r, _, callErr := procCredEnumerate.Call(0, flags, uintptr(unsafe.Pointer(&count)), uintptr(unsafe.Pointer(&buf)))
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
	r, _, _ := procCredDelete.Call(uintptr(unsafe.Pointer(name)), uintptr(credType), 0)
	if r == 0 {
		return errPurgeSecrets
	}
	return nil
}
