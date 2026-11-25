//go:build windows

package security

import (
	"fmt"
	"unsafe"

	"golang.org/x/sys/windows"
)

// ProtectSecret encrypts bytes using the current Windows user's DPAPI key.
func ProtectSecret(plain []byte) ([]byte, error) {
	if len(plain) == 0 {
		return []byte{}, nil
	}
	in := windows.DataBlob{
		Size: uint32(len(plain)),
		Data: &plain[0],
	}
	var out windows.DataBlob
	if err := windows.CryptProtectData(&in, nil, nil, 0, nil, windows.CRYPTPROTECT_UI_FORBIDDEN, &out); err != nil {
		return nil, fmt.Errorf("dpapi protect: %w", err)
	}
	defer windows.LocalFree(windows.Handle(unsafe.Pointer(out.Data)))
	res := unsafe.Slice(out.Data, out.Size)
	cp := make([]byte, len(res))
	copy(cp, res)
	return cp, nil
}

// UnprotectSecret decrypts bytes protected with ProtectSecret under the current Windows user.
func UnprotectSecret(cipher []byte) ([]byte, error) {
	if len(cipher) == 0 {
		return []byte{}, nil
	}
	in := windows.DataBlob{
		Size: uint32(len(cipher)),
		Data: &cipher[0],
	}
	var out windows.DataBlob
	if err := windows.CryptUnprotectData(&in, nil, nil, 0, nil, windows.CRYPTPROTECT_UI_FORBIDDEN, &out); err != nil {
		return nil, fmt.Errorf("dpapi unprotect: %w", err)
	}
	defer windows.LocalFree(windows.Handle(unsafe.Pointer(out.Data)))
	res := unsafe.Slice(out.Data, out.Size)
	cp := make([]byte, len(res))
	copy(cp, res)
	return cp, nil
}
