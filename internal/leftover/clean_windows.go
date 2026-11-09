//go:build windows

package leftover

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/registry"

	"appshelf/internal/runner"
)

// Outcome is what cleaning one find did.
type Outcome struct {
	ID    string
	OK    bool
	Error string
	// Backup is the file a registry find was exported to first.
	Backup string
}

// Clean removes the finds: files, directories and shortcuts go to the
// Recycle Bin; registry keys and values are exported to backupDir first,
// then deleted. Each find is checked again first: still there, and not
// protected. Others are the programs still installed.
func Clean(ctx context.Context, cfg Config, items []Item, others []Other, own string, backupDir string) []Outcome {
	out := make([]Outcome, 0, len(items))
	n := 0
	for _, it := range items {
		o := Outcome{ID: it.ID}
		var err error
		switch it.Kind {
		case Dir, Shortcut:
			switch {
			case Protected(cfg, it.Path, others, own):
				err = errors.New("这个位置受保护，没有清理")
			case !exists(it.Path):
				err = errors.New("已经不在了")
			default:
				err = recycle(it.Path)
			}
		case RegKey, RegValue:
			if !RegPathOK(cfg, it) {
				err = errors.New("这个注册表位置受保护，没有清理")
				break
			}
			n++
			o.Backup = filepath.Join(backupDir, fmt.Sprintf("%02d-%s.reg", n, fileName(it.Path+" "+it.Value)))
			err = cleanRegistry(ctx, it, o.Backup)
			if err != nil {
				o.Backup = ""
			}
		default:
			err = errors.New("未知的残留类型")
		}
		if err != nil {
			o.Error = err.Error()
		} else {
			o.OK = true
		}
		out = append(out, o)
	}
	return out
}

// RegPathOK reports a registry find Clean may delete: a value of the Run
// key, or a key below RegRoot that is neither the Run key nor holds it.
func RegPathOK(cfg Config, it Item) bool {
	p := strings.ToLower(strings.Trim(it.Path, `\`))
	if strings.Contains(p, `\\`) || strings.Contains(p, `..`) || p == "" {
		return false
	}
	run := strings.ToLower(cfg.RunKey)
	switch it.Kind {
	case RegValue:
		return run != "" && p == run && it.Value != ""
	case RegKey:
		root := strings.ToLower(cfg.RegRoot)
		if root == "" || !strings.HasPrefix(p, root+`\`) {
			return false
		}
		return run == "" || p != run && !strings.HasPrefix(run+`\`, p+`\`)
	}
	return false
}

func cleanRegistry(ctx context.Context, it Item, backup string) error {
	if err := os.MkdirAll(filepath.Dir(backup), 0o755); err != nil {
		return fmt.Errorf("无法创建备份目录：%w", err)
	}
	res, err := runner.Run(ctx, "reg.exe", []string{"export", `HKCU\` + it.Path, backup, "/y"}, runner.Options{Timeout: time.Minute})
	if err != nil {
		return fmt.Errorf("备份失败：%w", err)
	}
	if res.ExitCode != 0 {
		if !keyExists(it.Path) {
			return errors.New("已经不在了")
		}
		return fmt.Errorf("备份失败：%s", res.Tail(2))
	}
	if it.Kind == RegValue {
		k, err := registry.OpenKey(registry.CURRENT_USER, it.Path, registry.SET_VALUE)
		if err != nil {
			return fmt.Errorf("无法打开注册表：%w", err)
		}
		defer k.Close()
		if err := k.DeleteValue(it.Value); err != nil {
			return fmt.Errorf("删除失败：%w", err)
		}
		return nil
	}
	if err := deleteTree(it.Path); err != nil {
		return fmt.Errorf("删除失败：%w", err)
	}
	return nil
}

func keyExists(path string) bool {
	k, err := registry.OpenKey(registry.CURRENT_USER, path, registry.QUERY_VALUE)
	if err != nil {
		return false
	}
	k.Close()
	return true
}

var procRegDeleteTree = windows.NewLazySystemDLL("advapi32.dll").NewProc("RegDeleteTreeW")

// deleteTree deletes a key under HKEY_CURRENT_USER with its subkeys.
func deleteTree(path string) error {
	p, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return err
	}
	r, _, _ := procRegDeleteTree.Call(uintptr(windows.HKEY_CURRENT_USER), uintptr(unsafe.Pointer(p)))
	if r != 0 {
		return windows.Errno(r)
	}
	return nil
}

var procSHFileOperation = windows.NewLazySystemDLL("shell32.dll").NewProc("SHFileOperationW")

// shFileOpStruct is SHFILEOPSTRUCTW; Go aligns its fields as C does on
// 64-bit Windows; the 32-bit header packs it, so recycle refuses there.
type shFileOpStruct struct {
	hwnd                  uintptr
	wFunc                 uint32
	pFrom                 *uint16
	pTo                   *uint16
	fFlags                uint16
	fAnyOperationsAborted int32
	hNameMappings         uintptr
	lpszProgressTitle     *uint16
}

const (
	foDelete          = 0x0003
	fofSilent         = 0x0004
	fofNoConfirmation = 0x0010
	fofAllowUndo      = 0x0040
	fofNoErrorUI      = 0x0400
)

// recycle moves a file or directory to the Recycle Bin. Windows asks for
// administrator rights itself when the path needs them.
func recycle(path string) error {
	if unsafe.Sizeof(uintptr(0)) != 8 {
		return errors.New("32 位版本不支持移到回收站")
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		return err
	}
	// pFrom is a list of paths ended by two NULs.
	from, err := windows.UTF16FromString(abs)
	if err != nil {
		return err
	}
	from = append(from, 0)
	op := shFileOpStruct{
		wFunc:  foDelete,
		pFrom:  &from[0],
		fFlags: fofAllowUndo | fofNoConfirmation | fofSilent,
	}
	r, _, _ := procSHFileOperation.Call(uintptr(unsafe.Pointer(&op)))
	switch {
	case op.fAnyOperationsAborted != 0:
		return errors.New("已取消")
	case r != 0:
		return fmt.Errorf("无法移到回收站（错误 0x%X）", r)
	case exists(abs):
		return errors.New("无法移到回收站")
	}
	return nil
}

// fileName makes s fit in a file's name.
func fileName(s string) string {
	b := []rune{}
	for _, r := range s {
		if r < 0x20 || strings.ContainsRune(`<>:"/\|?* `, r) {
			r = '_'
		}
		b = append(b, r)
	}
	if len(b) > 60 {
		b = b[:60]
	}
	return strings.Trim(string(b), "._")
}
