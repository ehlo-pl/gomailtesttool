//go:build !windows

package msgraph

import "fmt"

func exportCertFromStore(thumbprint string) ([]byte, string, error) {
	return nil, "", fmt.Errorf("certificate store authentication is only supported on Windows")
}
