//go:build windows

package icons

import (
	"context"
	"os"
	"path/filepath"
	"strings"

	"golang.org/x/sys/windows/registry"
)

// MSI keeps its ARP icon separately from the Uninstall key. Resolve by
// product code, never by a similar application name.
func installerIcon(key string) string {
	product := key[strings.LastIndex(key, `\`)+1:]
	packed := packedProductCode(product)
	if packed == "" {
		return ""
	}
	for _, root := range []struct {
		key  registry.Key
		path string
	}{
		{registry.CURRENT_USER, `Software\Microsoft\Installer\Products\` + packed},
		{registry.CLASSES_ROOT, `Installer\Products\` + packed},
	} {
		k, err := registry.OpenKey(root.key, root.path, registry.QUERY_VALUE)
		if err != nil {
			continue
		}
		icon, _, _ := k.GetStringValue("ProductIcon")
		k.Close()
		if IconFile(icon) != "" {
			return icon
		}
	}
	return ""
}

// packageRoots reads the same AppModel repository used by UniGetUI's MSIX
// lookup, avoiding a PowerShell process and COM-dependent package enumeration.
func packageRoots(ctx context.Context) ([]packageRoot, error) {
	k, err := registry.OpenKey(registry.CURRENT_USER, `Software\Classes\Local Settings\Software\Microsoft\Windows\CurrentVersion\AppModel\Repository\Packages`, registry.READ)
	if err != nil {
		return nil, err
	}
	defer k.Close()
	names, err := k.ReadSubKeyNames(-1)
	if err != nil {
		return nil, err
	}
	var out []packageRoot
	for _, name := range names {
		if err := ctx.Err(); err != nil {
			return out, err
		}
		parts := strings.Split(name, "_")
		if len(parts) < 5 {
			continue
		}
		sub, err := registry.OpenKey(k, name, registry.QUERY_VALUE)
		if err != nil {
			continue
		}
		dir, _, _ := sub.GetStringValue("PackageRootFolder")
		sub.Close()
		if filepath.IsAbs(dir) {
			out = append(out, packageRoot{Family: parts[0] + "_" + parts[len(parts)-1], Dir: dir})
		}
		// Sparse packages can register an external executable directory while
		// their manifest and logos live in WindowsApps under the full identity.
		fallback := filepath.Join(os.Getenv("ProgramFiles"), "WindowsApps", name)
		if filepath.IsAbs(fallback) && !strings.EqualFold(filepath.Clean(dir), fallback) {
			out = append(out, packageRoot{Family: parts[0] + "_" + parts[len(parts)-1], Dir: fallback})
		}
	}
	return out, nil
}
