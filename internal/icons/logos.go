package icons

import (
	"context"
	"encoding/xml"
	"io"
	"os"
	"path/filepath"
	"strings"
)

type packageRoot struct{ Family, Dir string }

// PackageLogos reads local Appx manifests, including packages without a
// Start menu entry. Application logos take priority over generic store logos.
func PackageLogos(ctx context.Context) (map[string]string, error) {
	out := map[string]string{}
	packages, err := packageRoots(ctx)
	for _, p := range packages {
		if out[p.Family] != "" {
			continue
		}
		if ctx.Err() != nil {
			return out, ctx.Err()
		}
		f, e := os.Open(filepath.Join(p.Dir, "AppxManifest.xml"))
		if e != nil {
			continue
		}
		candidates := manifestLogos(io.LimitReader(f, 2<<20))
		f.Close()
		for _, logo := range candidates {
			if path := packageLogo(p.Dir, logo); path != "" {
				out[p.Family] = path
				break
			}
		}
	}
	return out, err
}

func manifestLogos(r io.Reader) []string {
	d := xml.NewDecoder(r)
	var applications, properties []string
	for {
		token, err := d.Token()
		if err != nil {
			if err != io.EOF {
				return nil
			}
			break
		}
		start, ok := token.(xml.StartElement)
		if !ok {
			continue
		}
		if start.Name.Local == "VisualElements" {
			for _, a := range start.Attr {
				if a.Name.Local == "Square44x44Logo" {
					applications = append(applications, a.Value)
				}
			}
		}
		if start.Name.Local == "Logo" {
			var logo string
			if d.DecodeElement(&logo, &start) == nil {
				properties = append(properties, strings.TrimSpace(logo))
			}
		}
	}
	return append(applications, properties...)
}

func packageLogo(dir, logo string) string {
	if !filepath.IsAbs(dir) || logo == "" || filepath.IsAbs(logo) || strings.Contains(logo, ":") {
		return ""
	}
	path := filepath.Join(dir, logo)
	rel, err := filepath.Rel(dir, path)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return ""
	}
	if !strings.EqualFold(filepath.Ext(path), ".png") {
		return ""
	}
	if fi, err := os.Stat(path); err == nil && !fi.IsDir() {
		return path
	}
	// Windows selects qualified assets such as StoreLogo.scale-200.png.
	stem := strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))
	for _, suffix := range []string{".targetsize-96_altform-unplated.png", ".targetsize-96.png", ".scale-200.png", ".scale-175.png", ".scale-150.png", ".scale-125.png", ".scale-100.png", ".targetsize-48_altform-unplated.png", ".targetsize-48.png", ".scale-400.png", ".scale-300.png"} {
		candidate := filepath.Join(filepath.Dir(path), stem+suffix)
		if fi, err := os.Stat(candidate); err == nil && !fi.IsDir() {
			return candidate
		}
	}
	return ""
}
