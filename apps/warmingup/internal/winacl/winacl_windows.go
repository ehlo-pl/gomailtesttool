//go:build windows

// Package winacl restricts files to administrators and one service account.
// Unix permission bits such as 0600 have no effect on Windows.
package winacl

import (
	"fmt"
	"strings"

	"golang.org/x/sys/windows"
)

// Restrict replaces the access list of path, without inheriting from its
// parent, by full control for SYSTEM and Administrators. A non-empty account
// other than LocalSystem is added with full control, or with read access when
// readOnly. On a directory the entries are inherited by its content.
func Restrict(path, account string, readOnly bool) error {
	name16, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return fmt.Errorf("%s: %w", path, err)
	}
	attrs, err := windows.GetFileAttributes(name16)
	if err != nil {
		return fmt.Errorf("%s: %w", path, err)
	}
	inherit := ""
	if attrs&windows.FILE_ATTRIBUTE_DIRECTORY != 0 {
		inherit = "OICI"
	}
	ace := func(rights, sid string) string { return "(A;" + inherit + ";" + rights + ";;;" + sid + ")" }
	sddl := "D:P" + ace("FA", "SY") + ace("FA", "BA")

	if name := strings.TrimPrefix(account, `.\`); name != "" && !strings.EqualFold(name, "LocalSystem") {
		sid, _, _, err := windows.LookupSID("", name)
		if err != nil {
			return fmt.Errorf("account %q: %w", account, err)
		}
		rights := "FA"
		if readOnly {
			rights = "FRFX"
		}
		sddl += ace(rights, sid.String())
	}

	sd, err := windows.SecurityDescriptorFromString(sddl)
	if err != nil {
		return fmt.Errorf("build access list: %w", err)
	}
	dacl, _, err := sd.DACL()
	if err != nil {
		return fmt.Errorf("build access list: %w", err)
	}
	if err := windows.SetNamedSecurityInfo(path, windows.SE_FILE_OBJECT,
		windows.DACL_SECURITY_INFORMATION|windows.PROTECTED_DACL_SECURITY_INFORMATION, nil, nil, dacl, nil); err != nil {
		return fmt.Errorf("set permissions of %s: %w", path, err)
	}
	return nil
}
