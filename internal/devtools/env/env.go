// Package env provides utilities for managing MSGRAPH* environment variables
// used by the Microsoft Graph integration tests and commands.
package env

import (
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/ehlo-pl/gomailtesttool/internal/common/security"
)

// RequiredVars are the environment variables needed for Microsoft Graph tests.
var RequiredVars = []string{
	"MSGRAPHTENANTID",
	"MSGRAPHCLIENTID",
	"MSGRAPHSECRET",
	"MSGRAPHMAILBOX",
}

// OptionalVars are the environment variables that may optionally be set.
var OptionalVars = []string{
	"MSGRAPHPROXY",
}

// VarStatus holds the name, value (masked), and whether a variable is set.
type VarStatus struct {
	Name   string
	Masked string
	Set    bool
}

// ShowVars writes the masked status of all MSGRAPH* variables to w.
func ShowVars(w io.Writer) {
	fmt.Fprintln(w, "MSGRAPH environment variables:")
	fmt.Fprintln(w)

	allVars := append(RequiredVars, OptionalVars...)
	for _, name := range allVars {
		val := os.Getenv(name)
		tag := "[required]"
		for _, o := range OptionalVars {
			if o == name {
				tag = "[optional]"
				break
			}
		}
		if val == "" {
			fmt.Fprintf(w, "  %-24s %s  (not set)\n", name, tag)
		} else {
			fmt.Fprintf(w, "  %-24s %s  %s\n", name, tag, maskVar(name, val))
		}
	}
}

// ClearCommands writes shell unset commands for all MSGRAPH* vars to w.
// The caller must execute these commands in their shell since a child process
// cannot modify its parent process's environment.
func ClearCommands(w io.Writer) {
	allVars := append(RequiredVars, OptionalVars...)
	for _, name := range allVars {
		fmt.Fprintf(w, "unset %s\n", name)
	}
}

// Missing returns the names of required variables that are not set.
func Missing() []string {
	var missing []string
	for _, name := range RequiredVars {
		if os.Getenv(name) == "" {
			missing = append(missing, name)
		}
	}
	return missing
}

// maskVar applies the appropriate masking function based on the variable name.
func maskVar(name, val string) string {
	switch {
	case strings.HasSuffix(name, "TENANTID") || strings.HasSuffix(name, "CLIENTID"):
		return security.MaskGUID(val)
	case strings.HasSuffix(name, "SECRET"):
		return security.MaskSecret(val)
	case strings.HasSuffix(name, "MAILBOX"):
		return security.MaskEmail(val)
	default:
		return security.MaskPassword(val)
	}
}
