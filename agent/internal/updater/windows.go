//go:build windows
// +build windows

package updater

import (
	"context"
	"encoding/json"
	"os"
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

// psWindowsUpdateVersion is the PSWindowsUpdate release installed when the
// module is missing.
const psWindowsUpdateVersion = "2.2.1.5"

// applyUpdatesScript installs updates with PSWindowsUpdate. The package list
// is passed as data through PATCHLI_PACKAGES_JSON, never as script text, and
// the module is only loaded if its Authenticode signature is valid and was
// issued to the module's publisher.
const applyUpdatesScript = `
$ErrorActionPreference = 'Stop'
if (!(Get-Module -ListAvailable -Name PSWindowsUpdate)) {
	Install-Module -Name PSWindowsUpdate -Repository PSGallery -RequiredVersion '` + psWindowsUpdateVersion + `' -Force -AcceptLicense
}
$module = Get-Module -ListAvailable -Name PSWindowsUpdate | Sort-Object Version -Descending | Select-Object -First 1
foreach ($file in @('PSWindowsUpdate.dll', 'PSWindowsUpdate.psd1')) {
	$sig = Get-AuthenticodeSignature -FilePath (Join-Path $module.ModuleBase $file)
	if ($sig.Status -ne 'Valid' -or $sig.SignerCertificate.Subject -notlike 'CN=PowerClouds Michal Gajda,*') {
		throw "Untrusted PSWindowsUpdate module signature on $file"
	}
}
Import-Module (Join-Path $module.ModuleBase 'PSWindowsUpdate.psd1')
$pkgNames = @($env:PATCHLI_PACKAGES_JSON | ConvertFrom-Json)
if ($pkgNames.Count -gt 0 -and $pkgNames[0] -ne "") {
	Install-WindowsUpdate -Title $pkgNames -AcceptAll -IgnoreReboot
} else {
	Install-WindowsUpdate -AcceptAll -IgnoreReboot
}
`

func (m *WindowsManager) ApplyUpdates(ctx context.Context, packages []string) (UpdateResult, error) {
	pkgBytes, _ := json.Marshal(packages)
	cmd := execCommandContext(ctx, "powershell", "-NoProfile", "-NonInteractive", "-Command", applyUpdatesScript)
	if cmd.Env == nil {
		cmd.Env = os.Environ()
	}
	cmd.Env = append(cmd.Env, "PATCHLI_PACKAGES_JSON="+string(pkgBytes))
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
	if err := checkDiskSpaceFunc("C:", 5368709120); err != nil {
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
