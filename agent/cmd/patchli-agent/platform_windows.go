//go:build windows

package main

import (
	"os/exec"
	"path/filepath"
	"syscall"

	"golang.org/x/sys/windows"
)

func restrictTokenPath(path string, directory bool) error {
	user, err := windows.GetCurrentProcessToken().GetTokenUser()
	if err != nil {
		return err
	}
	inheritance := ""
	if directory {
		inheritance = "OICI"
	}
	// Protected DACL: only this service account, SYSTEM and administrators can
	// read tokens. Chmod alone does not restrict Windows file access.
	sddl := "D:P(A;" + inheritance + ";FA;;;SY)(A;" + inheritance + ";FA;;;BA)(A;" + inheritance + ";FA;;;" + user.User.Sid.String() + ")"
	descriptor, err := windows.SecurityDescriptorFromString(sddl)
	if err != nil {
		return err
	}
	acl, _, err := descriptor.DACL()
	if err != nil {
		return err
	}
	return windows.SetNamedSecurityInfo(path, windows.SE_FILE_OBJECT, windows.DACL_SECURITY_INFORMATION|windows.PROTECTED_DACL_SECURITY_INFORMATION, nil, nil, acl, nil)
}

func restartWindowsAgent() ([]byte, error) {
	systemDir, err := windows.GetSystemDirectory()
	if err != nil {
		return nil, err
	}
	executable := filepath.Join(systemDir, "WindowsPowerShell", "v1.0", "powershell.exe")
	// The script is constant. Neither PATH-derived executables nor dynamic data
	// are interpolated into a shell command. The detached helper survives stop.
	// #nosec G204 -- Windows system directory API selects PowerShell; all arguments and the service name are constant.
	helper := exec.Command(executable, "-NoProfile", "-NonInteractive", "-Command", "$ErrorActionPreference='Stop'; Start-Sleep -Seconds 2; Restart-Service -Name patchli-agent")
	helper.SysProcAttr = &syscall.SysProcAttr{CreationFlags: windows.DETACHED_PROCESS | windows.CREATE_NO_WINDOW, HideWindow: true}
	if err := helper.Start(); err != nil {
		return nil, err
	}
	if err := helper.Process.Release(); err != nil {
		return nil, err
	}
	return []byte("Restart scheduled via detached Windows service helper"), nil
}
