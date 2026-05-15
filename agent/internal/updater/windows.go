//go:build windows
// +build windows

package updater

import (
	"context"
	"strings"
)

// WindowsManager implements the PackageManager interface using PowerShell and Windows Update Agent API.
type WindowsManager struct{}

func (m *WindowsManager) CheckUpdates(ctx context.Context) (UpdateResult, error) {
	script := `
$UpdateSession = New-Object -ComObject Microsoft.Update.Session
$UpdateSearcher = $UpdateSession.CreateUpdateSearcher()
$SearchResult = $UpdateSearcher.Search("IsInstalled=0")
$SearchResult.Updates.Count
`
	cmd := execCommandContext(ctx, "powershell", "-NoProfile", "-Command", script)
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
	cmd := execCommandContext(ctx, "powershell", "-NoProfile", "-Command", script)
	out, err := cmd.CombinedOutput()
	return UpdateResult{Success: err == nil, Output: string(out), Error: err}, err
}

func (m *WindowsManager) RebootRequired() bool {
	script := `
$sysInfo = New-Object -ComObject "Microsoft.Update.SystemInfo"
$sysInfo.RebootRequired
`
	cmd := execCommand("powershell", "-NoProfile", "-Command", script)
	out, err := cmd.Output()
	return err == nil && strings.TrimSpace(string(out)) == "True"
}

func (m *WindowsManager) PreFlightCheck(ctx context.Context) error {
	// Check for 5GB free on C:
	if err := CheckDiskSpace("C:", 5368709120); err != nil {
		return err
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
	return execCommandContext(ctx, "powershell", "-NoProfile", "-Command", script).Run()
}
