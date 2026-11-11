//go:build !windows

// Package svc hosts testmailflow as a native Windows service. On other
// systems run "testmailflow run" under systemd or another supervisor.
package svc

import (
	"context"
	"errors"
)

// Supported reports whether native service hosting is available.
const Supported = false

var errUnsupported = errors.New("native service management is available on Windows only; run 'testmailflow run' under systemd or another supervisor")

// IsService always reports false outside Windows.
func IsService() bool { return false }

// Run is unsupported outside Windows.
func Run(string, func(ctx context.Context) error) error { return errUnsupported }

// InstallOptions describe the service to create.
type InstallOptions struct {
	Name, DisplayName, Description string
	ExePath                        string
	Args                           []string
	User, Password                 string
}

// Install is unsupported outside Windows.
func Install(InstallOptions) error { return errUnsupported }

// Uninstall is unsupported outside Windows.
func Uninstall(string) error { return errUnsupported }

// Start is unsupported outside Windows.
func Start(string) error { return errUnsupported }

// Stop is unsupported outside Windows.
func Stop(string) error { return errUnsupported }

// State is unsupported outside Windows.
func State(string) (string, error) { return "", errUnsupported }
