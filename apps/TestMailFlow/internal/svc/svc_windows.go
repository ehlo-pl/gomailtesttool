//go:build windows

// Package svc hosts testmailflow as a native Windows service and manages
// its installation through the service control manager.
package svc

import (
	"context"
	"errors"
	"fmt"
	"time"

	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/svc"
	"golang.org/x/sys/windows/svc/mgr"
)

// Supported reports whether native service hosting is available.
const Supported = true

// IsService reports whether the process was started by the service control manager.
func IsService() bool {
	ok, err := svc.IsWindowsService()
	return err == nil && ok
}

type handler struct {
	run func(ctx context.Context) error
	err error
}

func (h *handler) Execute(_ []string, requests <-chan svc.ChangeRequest, status chan<- svc.Status) (bool, uint32) {
	status <- svc.Status{State: svc.StartPending}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	go func() { done <- h.run(ctx) }()
	status <- svc.Status{State: svc.Running, Accepts: svc.AcceptStop | svc.AcceptShutdown}

	for {
		select {
		case err := <-done:
			h.err = err
			status <- svc.Status{State: svc.StopPending}
			if err != nil {
				return true, 1 // service-specific exit code: SCM applies the recovery actions
			}
			return false, 0
		case req := <-requests:
			switch req.Cmd {
			case svc.Interrogate:
				status <- req.CurrentStatus
			case svc.Stop, svc.Shutdown:
				status <- svc.Status{State: svc.StopPending, WaitHint: 20000}
				cancel()
				select {
				case h.err = <-done:
				case <-time.After(20 * time.Second):
					h.err = errors.New("timed out waiting for the engine to stop")
				}
				return false, 0
			}
		}
	}
}

// Run executes run under the service control manager and returns when the
// service is stopped. run must return when its context is cancelled.
func Run(name string, run func(ctx context.Context) error) error {
	h := &handler{run: run}
	if err := svc.Run(name, h); err != nil {
		return fmt.Errorf("service %s: %w", name, err)
	}
	return h.err
}

// InstallOptions describe the service to create.
type InstallOptions struct {
	Name, DisplayName, Description string
	ExePath                        string
	Args                           []string
	User, Password                 string // empty User = LocalSystem
}

// Install creates the service (automatic delayed start, restart on failure).
func Install(o InstallOptions) error {
	m, err := mgr.Connect()
	if err != nil {
		return fmt.Errorf("connect to service manager (administrator rights required): %w", err)
	}
	defer func() { _ = m.Disconnect() }()
	if s, err := m.OpenService(o.Name); err == nil {
		_ = s.Close()
		return fmt.Errorf("service %s already exists; run 'service uninstall' first", o.Name)
	}
	s, err := m.CreateService(o.Name, o.ExePath, mgr.Config{
		DisplayName:      o.DisplayName,
		Description:      o.Description,
		StartType:        mgr.StartAutomatic,
		DelayedAutoStart: true,
		ServiceStartName: o.User,
		Password:         o.Password,
	}, o.Args...)
	if err != nil {
		return fmt.Errorf("create service: %w", err)
	}
	defer func() { _ = s.Close() }()
	// Restart after 1, 5 and 15 minutes; reset the failure counter after a day.
	if err := s.SetRecoveryActions([]mgr.RecoveryAction{
		{Type: mgr.ServiceRestart, Delay: time.Minute},
		{Type: mgr.ServiceRestart, Delay: 5 * time.Minute},
		{Type: mgr.ServiceRestart, Delay: 15 * time.Minute},
	}, 86400); err != nil {
		return fmt.Errorf("service created, but setting recovery actions failed: %w", err)
	}
	return nil
}

func open(name string) (*mgr.Mgr, *mgr.Service, error) {
	m, err := mgr.Connect()
	if err != nil {
		return nil, nil, fmt.Errorf("connect to service manager (administrator rights required): %w", err)
	}
	s, err := m.OpenService(name)
	if err != nil {
		_ = m.Disconnect()
		if errors.Is(err, windows.ERROR_SERVICE_DOES_NOT_EXIST) {
			return nil, nil, fmt.Errorf("service %s is not installed", name)
		}
		return nil, nil, fmt.Errorf("open service %s: %w", name, err)
	}
	return m, s, nil
}

// Uninstall stops (if needed) and deletes the service.
func Uninstall(name string) error {
	m, s, err := open(name)
	if err != nil {
		return err
	}
	defer func() { _ = m.Disconnect() }()
	defer func() { _ = s.Close() }()
	if st, err := s.Query(); err == nil && st.State != svc.Stopped {
		_ = stopAndWait(s)
	}
	if err := s.Delete(); err != nil {
		return fmt.Errorf("delete service: %w", err)
	}
	return nil
}

// Start starts the service.
func Start(name string) error {
	m, s, err := open(name)
	if err != nil {
		return err
	}
	defer func() { _ = m.Disconnect() }()
	defer func() { _ = s.Close() }()
	if err := s.Start(); err != nil {
		return fmt.Errorf("start service: %w", err)
	}
	return nil
}

// Stop stops the service and waits until it has stopped.
func Stop(name string) error {
	m, s, err := open(name)
	if err != nil {
		return err
	}
	defer func() { _ = m.Disconnect() }()
	defer func() { _ = s.Close() }()
	return stopAndWait(s)
}

func stopAndWait(s *mgr.Service) error {
	st, err := s.Control(svc.Stop)
	if err != nil {
		if errors.Is(err, windows.ERROR_SERVICE_NOT_ACTIVE) {
			return nil
		}
		return fmt.Errorf("stop service: %w", err)
	}
	deadline := time.Now().Add(30 * time.Second)
	for st.State != svc.Stopped {
		if time.Now().After(deadline) {
			return errors.New("timed out waiting for the service to stop")
		}
		time.Sleep(300 * time.Millisecond)
		if st, err = s.Query(); err != nil {
			return fmt.Errorf("query service: %w", err)
		}
	}
	return nil
}

// State returns the service state as text.
func State(name string) (string, error) {
	m, s, err := open(name)
	if err != nil {
		return "", err
	}
	defer func() { _ = m.Disconnect() }()
	defer func() { _ = s.Close() }()
	st, err := s.Query()
	if err != nil {
		return "", fmt.Errorf("query service: %w", err)
	}
	switch st.State {
	case svc.Stopped:
		return "stopped", nil
	case svc.StartPending:
		return "starting", nil
	case svc.StopPending:
		return "stopping", nil
	case svc.Running:
		return "running", nil
	case svc.Paused, svc.PausePending, svc.ContinuePending:
		return "paused", nil
	}
	return fmt.Sprintf("state %d", st.State), nil
}
