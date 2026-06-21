//go:build windows

package msgraph

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"syscall"
	"unsafe"
)

var (
	modcrypt32 = syscall.NewLazyDLL("crypt32.dll")

	procCertOpenSystemStoreW             = modcrypt32.NewProc("CertOpenSystemStoreW")
	procCertFindCertificateInStore       = modcrypt32.NewProc("CertFindCertificateInStore")
	procCertOpenStore                    = modcrypt32.NewProc("CertOpenStore")
	procCertAddCertificateContextToStore = modcrypt32.NewProc("CertAddCertificateContextToStore")
	procCertCloseStore                   = modcrypt32.NewProc("CertCloseStore")
	procPFXExportCertStoreEx             = modcrypt32.NewProc("PFXExportCertStoreEx")
	procCertFreeCertificateContext       = modcrypt32.NewProc("CertFreeCertificateContext")
)

const (
	X509_ASN_ENCODING   = 0x00000001
	PKCS_7_ASN_ENCODING = 0x00010000

	CERT_FIND_SHA1_HASH = 1 << 16 // 65536

	CERT_STORE_PROV_MEMORY = 2

	CERT_STORE_ADD_ALWAYS = 4

	EXPORT_PRIVATE_KEYS                   = 0x0004
	REPORT_NO_PRIVATE_KEY                 = 0x0008
	REPORT_NOT_ABLE_TO_EXPORT_PRIVATE_KEY = 0x0010
)

type CryptoApiBlob struct {
	DataSize uint32
	Data     *byte
}

func exportCertFromStore(thumbprintStr string) ([]byte, string, error) {
	// Validate thumbprint format (SHA1 hash = 40 hex characters = 20 bytes)
	if len(thumbprintStr) != 40 {
		return nil, "", fmt.Errorf("thumbprint must be exactly 40 characters (SHA1 hash), got %d", len(thumbprintStr))
	}

	// Validate that thumbprint contains only hexadecimal characters
	if !isHexString(thumbprintStr) {
		return nil, "", fmt.Errorf("thumbprint must contain only hexadecimal characters (0-9, a-f, A-F)")
	}

	// 1. Decode thumbprint hex to bytes for the search blob
	thumbprintBytes, err := hex.DecodeString(thumbprintStr)
	if err != nil {
		return nil, "", fmt.Errorf("invalid thumbprint format: %w", err)
	}

	// 2. Open "Current User" -> "My" store
	// CertOpenSystemStoreW(0, "MY")
	storeNamePtr, _ := syscall.UTF16PtrFromString("MY")
	hSourceStore, _, err := procCertOpenSystemStoreW.Call(
		0,
		uintptr(unsafe.Pointer(storeNamePtr)),
	)
	if hSourceStore == 0 {
		return nil, "", fmt.Errorf("failed to open system certificate store: %v", err)
	}
	defer procCertCloseStore.Call(hSourceStore, 0)

	// 3. Find the certificate by SHA1 Hash
	hashBlob := CryptoApiBlob{
		DataSize: uint32(len(thumbprintBytes)),
		Data:     &thumbprintBytes[0],
	}

	pCertContext, _, _ := procCertFindCertificateInStore.Call(
		hSourceStore,
		uintptr(X509_ASN_ENCODING|PKCS_7_ASN_ENCODING),
		0,
		uintptr(CERT_FIND_SHA1_HASH),
		uintptr(unsafe.Pointer(&hashBlob)),
		0, // pPrevCertContext
	)

	if pCertContext == 0 {
		return nil, "", fmt.Errorf("certificate with thumbprint %s not found", thumbprintStr)
	}
	defer procCertFreeCertificateContext.Call(pCertContext)

	// 4. Create a temporary memory store
	hTempStore, _, err := procCertOpenStore.Call(
		uintptr(CERT_STORE_PROV_MEMORY),
		0, 0, 0, 0,
	)
	if hTempStore == 0 {
		return nil, "", fmt.Errorf("failed to create temporary memory store: %v", err)
	}
	defer procCertCloseStore.Call(hTempStore, 0)

	// 5. Add the certificate to the memory store
	// This makes sure we only export this specific certificate
	ret, _, err := procCertAddCertificateContextToStore.Call(
		hTempStore,
		pCertContext,
		uintptr(CERT_STORE_ADD_ALWAYS),
		0,
	)
	if ret == 0 {
		return nil, "", fmt.Errorf("failed to add certificate to memory store: %v", err)
	}

	// 6. Generate a random password for the PFX
	password, err := generateRandomPassword()
	if err != nil {
		return nil, "", err
	}
	pwPtr, _ := syscall.UTF16PtrFromString(password)

	// 7. Export to PFX Blob
	// First call to get the size
	var blob CryptoApiBlob
	exportFlags := uintptr(EXPORT_PRIVATE_KEYS | REPORT_NO_PRIVATE_KEY | REPORT_NOT_ABLE_TO_EXPORT_PRIVATE_KEY)

	ret, _, err = procPFXExportCertStoreEx.Call(
		hTempStore,
		uintptr(unsafe.Pointer(&blob)),
		uintptr(unsafe.Pointer(pwPtr)),
		0, // reserved
		exportFlags,
	)
	if ret == 0 {
		return nil, "", fmt.Errorf("failed to determine PFX size (key might not be exportable): %v", err)
	}

	// Allocate buffer
	buf := make([]byte, blob.DataSize)
	blob.Data = &buf[0]

	// Second call to get the data
	ret, _, err = procPFXExportCertStoreEx.Call(
		hTempStore,
		uintptr(unsafe.Pointer(&blob)),
		uintptr(unsafe.Pointer(pwPtr)),
		0,
		exportFlags,
	)
	if ret == 0 {
		return nil, "", fmt.Errorf("failed to export PFX data: %v", err)
	}

	return buf, password, nil
}

func generateRandomPassword() (string, error) {
	b := make([]byte, 16)
	_, err := rand.Read(b)
	if err != nil {
		return "", fmt.Errorf("failed to generate random password: %w", err)
	}
	return hex.EncodeToString(b), nil
}

// isHexString validates that a string contains only hexadecimal characters (0-9, a-f, A-F)
func isHexString(s string) bool {
	for _, c := range s {
		if !((c >= '0' && c <= '9') || (c >= 'a' && c <= 'f') || (c >= 'A' && c <= 'F')) {
			return false
		}
	}
	return true
}
