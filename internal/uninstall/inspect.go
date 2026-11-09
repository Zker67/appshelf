package uninstall

import (
	"context"
	"path/filepath"
	"strings"

	"appshelf/internal/detect"
	"appshelf/internal/procs"
)

// Inspection is a plan with what the machine says now: programs running,
// whether an app package may be removed, and who signed the uninstaller.
type Inspection struct {
	Plan
	// Signer is who signed the program's uninstaller or main program, ""
	// when unsigned or unknown; Signed means the signature is valid.
	Signer string
	Signed bool
	// SignedFile is the file whose signature was read.
	SignedFile string
}

// inspectScript reads, in one PowerShell, the app packages' properties and
// the files' signatures.
const inspectScript = `
$store = @()
foreach ($s in @($P.Store)) {
  if (-not $s) { continue }
  $pkg = $null
  try { $pkg = Get-AppxPackage -Name ([string]$s.Name) | Where-Object { $_.PackageFamilyName -eq [string]$s.Family } | Select-Object -First 1 } catch {}
  if ($pkg) {
    $store += [pscustomobject]@{ Family = [string]$s.Family; Found = $true; Framework = [bool]$pkg.IsFramework; NonRemovable = [bool]$pkg.NonRemovable; System = ([string]$pkg.SignatureKind -eq 'System') }
  } else {
    $store += [pscustomobject]@{ Family = [string]$s.Family; Found = $false; Framework = $false; NonRemovable = $false; System = $false }
  }
}
$files = @()
foreach ($f in @($P.Files)) {
  if (-not $f) { continue }
  $signer = ''
  $status = ''
  try {
    $sig = Get-AuthenticodeSignature -LiteralPath ([string]$f) -ErrorAction Stop
    $status = [string]$sig.Status
    if ($sig.SignerCertificate) { $signer = $sig.SignerCertificate.GetNameInfo([System.Security.Cryptography.X509Certificates.X509NameType]::SimpleName, $false) }
  } catch {}
  $files += [pscustomobject]@{ Path = [string]$f; Status = $status; Signer = [string]$signer }
}
Write-Result @{ Store = $store; Files = $files }
`

type storeInfo struct {
	Family                         string
	Found, Framework, NonRemovable bool
	System                         bool
}

type fileInfo struct {
	Path, Status, Signer string
}

// Inspect plans the removal of each target and checks the machine: a
// program running from its directory, an app package that is a framework
// or part of Windows, the uninstaller's signature.
func (r *Runner) Inspect(ctx context.Context, targets []Target) []Inspection {
	out := make([]Inspection, len(targets))
	stores := []map[string]string{}
	files := []string{}
	for i, t := range targets {
		out[i] = Inspection{Plan: PlanFor(t, false, r.opt)}
		if out[i].Blocked != "" {
			continue
		}
		if t.Method == ByStore {
			stores = append(stores, map[string]string{"Name": strings.SplitN(t.PackageFullName, "_", 2)[0], "Family": familyOf(t.PackageFullName)})
		}
		if f := signedFile(t, out[i].Plan); f != "" {
			out[i].SignedFile = f
			files = append(files, f)
		}
	}
	var got struct {
		Store []storeInfo
		Files []fileInfo
	}
	if len(stores) > 0 || len(files) > 0 {
		if err := r.powershell(ctx, inspectScript, map[string]any{"Store": stores, "Files": files}, &got, nil); err != nil {
			for i, t := range targets {
				if t.Method == ByStore && out[i].Blocked == "" {
					out[i].Blocked = "无法读取这个应用包：" + err.Error()
				}
			}
		}
	}
	byFamily := map[string]storeInfo{}
	for _, s := range got.Store {
		byFamily[strings.ToLower(s.Family)] = s
	}
	byFile := map[string]fileInfo{}
	for _, f := range got.Files {
		byFile[strings.ToLower(f.Path)] = f
	}
	paths, _ := r.processes()
	for i, t := range targets {
		in := &out[i]
		if in.Blocked != "" {
			continue
		}
		if t.Method == ByStore {
			if s, ok := byFamily[strings.ToLower(familyOf(t.PackageFullName))]; ok {
				switch {
				case !s.Found:
					in.Blocked = "找不到这个应用包，可能已经卸载"
				case s.Framework:
					in.Blocked = "这是其他应用依赖的框架包"
				case s.System || s.NonRemovable:
					in.Blocked = "这是 Windows 自带的应用，不能卸载"
				}
			}
		}
		if f, ok := byFile[strings.ToLower(in.SignedFile)]; ok {
			in.Signer, in.Signed = f.Signer, f.Status == "Valid"
		}
		if t.Location != "" && detect.OwnDirectory(t.Location) && procs.Under(paths, t.Location) {
			in.Notes = append([]string{"程序正在运行，卸载前请先关闭它"}, in.Notes...)
		}
	}
	return out
}

// signedFile is the file whose signature tells who made t: its
// uninstaller, else its main program.
func signedFile(t Target, p Plan) string {
	if p.File != "" && !strings.EqualFold(filepath.Base(p.File), "msiexec.exe") && strings.EqualFold(filepath.Ext(p.File), ".exe") {
		return p.File
	}
	if t.Reg == nil {
		return ""
	}
	for _, cmd := range []string{t.Reg.DisplayIcon, t.Reg.UninstallString} {
		f := detect.CommandFile(cmd)
		if strings.EqualFold(filepath.Ext(f), ".exe") && !strings.EqualFold(filepath.Base(f), "msiexec.exe") && filepath.IsAbs(f) {
			return f
		}
	}
	return ""
}
