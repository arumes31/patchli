//go:build windows
// +build windows

package updater

import (
	"context"
	"fmt"
	"os/exec"
	"strings"
)

// WindowsManager implements the PackageManager interface using PowerShell and Windows Update Agent API.
type WindowsManager struct{}

func (m *WindowsManager) CheckUpdates(ctx context.Context) (UpdateResult, error) {
	// A simple implementation using PowerShell to query WUA.
	// In production, this would use a dedicated Go library wrapping the COM object.
	script := `
$UpdateSession = New-Object -ComObject Microsoft.Update.Session
$UpdateSearcher = $UpdateSession.CreateUpdateSearcher()
$SearchResult = $UpdateSearcher.Search("IsInstalled=0")
$SearchResult.Updates.Count
`
	cmd := exec.CommandContext(ctx, "powershell", "-NoProfile", "-Command", script)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return UpdateResult{Success: false, Output: string(out), Error: err}, err
	}

	return UpdateResult{Success: true, Output: string(out), Error: nil}, nil
}

func (m *WindowsManager) ApplyUpdates(ctx context.Context, packages []string) (UpdateResult, error) {
	script := `
if (!(Get-Module -ListAvailable -Name PSWindowsUpdate)) {
	Install-Module -Name PSWindowsUpdate -Force -SkipPublisherCheck -AcceptLicense
}
Import-Module PSWindowsUpdate
$pkgNames = @("` + strings.Join(packages, `", "`) + `")
if ($pkgNames.Length -gt 0 -and $pkgNames[0] -ne "") {
	Install-WindowsUpdate -Title $pkgNames -AcceptAll -IgnoreReboot
} else {
	Install-WindowsUpdate -AcceptAll -IgnoreReboot
}
`
	cmd := exec.CommandContext(ctx, "powershell", "-NoProfile", "-Command", script)
	out, err := cmd.CombinedOutput()
	return UpdateResult{Success: err == nil, Output: string(out), Error: err}, err
}

func (m *WindowsManager) RebootRequired() bool {
	// Checking the PendingFileRenameOperations registry key or WU reboot required flag.
	script := `
$sysInfo = New-Object -ComObject "Microsoft.Update.SystemInfo"
$sysInfo.RebootRequired
`
	cmd := exec.Command("powershell", "-NoProfile", "-Command", script)
	out, err := cmd.Output()
	return err == nil && string(out) == "True\r\n"
}

func (m *WindowsManager) PreFlightCheck(ctx context.Context) error {
	// Check for 5GB free on C:
	script := `
$disk = Get-WmiObject Win32_LogicalDisk -Filter "DeviceID='C:'"
if ($disk.FreeSpace -lt 5368709120) { exit 1 }
`
	cmd := exec.CommandContext(ctx, "powershell", "-NoProfile", "-Command", script)
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("insufficient disk space on C:")
	}
	return nil
}

func (m *WindowsManager) Cleanup(ctx context.Context) error {
	script := `
$ErrorActionPreference = 'Stop'
try {
	Stop-Service wuauserv -Force
	Remove-Item -Recurse -Force "C:\Windows\SoftwareDistribution\Download\*"
} finally {
	Start-Service wuauserv
}
`
	return exec.CommandContext(ctx, "powershell", "-NoProfile", "-Command", script).Run()
}
