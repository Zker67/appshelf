package runner

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

// prelude starts every script. The script's input arrives as $P, read
// from a JSON file, so no value goes through the command line, where
// PowerShell would take one starting with "-" for a parameter. The script
// answers with Write-Result; an error ends it with its message in a file,
// written as UTF-8, which the console of Windows PowerShell is not.
const prelude = `param([string]$In, [string]$Out)
$ErrorActionPreference = 'Stop'
$ProgressPreference = 'SilentlyContinue'
# Go inherits the launcher's PSModulePath without PowerShell 7's native-command
# adjustment. Restore this Windows PowerShell host's standard locations locally.
$modulePaths = @()
$documents = [Environment]::GetFolderPath('MyDocuments')
if ($documents) { $modulePaths += Join-Path $documents 'WindowsPowerShell\Modules' }
if ($env:ProgramFiles) { $modulePaths += Join-Path $env:ProgramFiles 'WindowsPowerShell\Modules' }
$modulePaths += Join-Path $PSHOME 'Modules'
$modulePaths += @($env:PSModulePath -split ';' | Where-Object { $_ })
$env:PSModulePath = ($modulePaths | Select-Object -Unique) -join ';'
$utf8 = New-Object System.Text.UTF8Encoding $false
function Write-Result($value) {
  $json = ConvertTo-Json -InputObject $value -Depth 8 -Compress
  [System.IO.File]::WriteAllText($Out, $json, $utf8)
}
function Write-Failure([string]$message) {
  [System.IO.File]::WriteAllText($Out + '.err', $message, $utf8)
}
trap {
  Write-Failure $_.Exception.Message
  exit 1
}
$P = Get-Content -Raw -LiteralPath $In -Encoding UTF8 | ConvertFrom-Json
`

// ScriptError is a script that ended with an error.
type ScriptError struct {
	ExitCode int
	Message  string // the script's message, or the end of its output
}

func (e *ScriptError) Error() string {
	if e.Message == "" {
		return fmt.Sprintf("PowerShell 脚本以退出码 %d 结束", e.ExitCode)
	}
	return e.Message
}

// PowerShellPath returns Windows PowerShell 5.1, which every Windows 10
// and 11 has, from the system directory rather than from PATH.
func PowerShellPath() string {
	if root := os.Getenv("SystemRoot"); root != "" {
		p := filepath.Join(root, "System32", "WindowsPowerShell", "v1.0", "powershell.exe")
		if _, err := os.Stat(p); err == nil {
			return p
		}
	}
	return "powershell.exe"
}

// PowerShell runs body, PowerShell code, in Windows PowerShell after the
// prelude, with input as $P. The value the script passes to Write-Result
// decodes into out, when out is not nil. A script that exits with another
// code than 0 returns a *ScriptError, with the Result.
func PowerShell(ctx context.Context, body string, input, out any, opt Options) (Result, error) {
	dir, err := os.MkdirTemp("", "appshelf-ps-")
	if err != nil {
		return Result{ExitCode: -1}, err
	}
	defer os.RemoveAll(dir)

	if input == nil {
		input = struct{}{}
	}
	in, err := json.Marshal(input)
	if err != nil {
		return Result{ExitCode: -1}, err
	}
	inPath := filepath.Join(dir, "in.json")
	outPath := filepath.Join(dir, "out.json")
	scriptPath := filepath.Join(dir, "script.ps1")
	if err := os.WriteFile(inPath, in, 0o600); err != nil {
		return Result{ExitCode: -1}, err
	}
	// Windows PowerShell reads a script without a byte order mark in the
	// system's code page: the scripts are ASCII, and the mark keeps any
	// other text right.
	if err := os.WriteFile(scriptPath, append([]byte("\xef\xbb\xbf"), prelude+body...), 0o600); err != nil {
		return Result{ExitCode: -1}, err
	}
	args := []string{"-NoProfile", "-NonInteractive", "-ExecutionPolicy", "Bypass", "-File", scriptPath, "-In", inPath, "-Out", outPath}
	res, err := Run(ctx, PowerShellPath(), args, opt)
	if err != nil {
		return res, err
	}
	if res.ExitCode != 0 {
		msg, rerr := os.ReadFile(outPath + ".err")
		if rerr != nil {
			msg = []byte(res.Tail(3))
		}
		return res, &ScriptError{ExitCode: res.ExitCode, Message: strings.TrimSpace(string(msg))}
	}
	if out == nil {
		return res, nil
	}
	data, err := os.ReadFile(outPath)
	if errors.Is(err, fs.ErrNotExist) {
		return res, errors.New("PowerShell 脚本没有返回结果")
	}
	if err != nil {
		return res, err
	}
	if err := json.Unmarshal(data, out); err != nil {
		return res, fmt.Errorf("PowerShell 脚本的结果无法解析：%w", err)
	}
	return res, nil
}
