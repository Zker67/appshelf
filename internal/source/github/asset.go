package github

import (
	"errors"
	"fmt"
	"regexp"
	"sort"
	"strings"
)

// Asset is a file of a release.
type Asset struct {
	Name   string `json:"name"`
	Size   int64  `json:"size"`
	Digest string `json:"digest"` // "sha256:…", when GitHub computed it
	URL    string `json:"browser_download_url"`
}

// Architectures as asset names write them, checked in this order: x86_64
// contains x86. A boundary is anything but a letter or a digit, as the _
// and - of "gh_2.1_windows_amd64.msi".
var (
	archAMD64 = regexp.MustCompile(`(?:^|[^a-z0-9])(?:x86_64|x86-64|amd64|x64|win64)(?:[^a-z0-9]|$)`)
	archARM64 = regexp.MustCompile(`(?:^|[^a-z0-9])(?:aarch64|arm64)(?:[^a-z0-9]|$)`)
	arch386   = regexp.MustCompile(`(?:^|[^a-z0-9])(?:i[3-6]86|x86|win32|386|ia32)(?:[^a-z0-9]|$)`)
	noise     = regexp.MustCompile(`(?:^|[^a-z0-9])(?:debug|symbols|pdb)(?:[^a-z0-9]|$)`)
)

// archOf returns the architecture an asset name says, "" for none.
func archOf(name string) string {
	n := strings.ToLower(name)
	switch {
	case archAMD64.MatchString(n):
		return "amd64"
	case archARM64.MatchString(n):
		return "arm64"
	case arch386.MatchString(n):
		return "386"
	}
	return ""
}

// archScore rates an asset's architecture for the machine's, as GOARCH
// writes it: the same is best, none said next, one the machine runs by
// emulation last; -1 cannot run.
func archScore(asset, machine string) int {
	if asset == "" {
		return 2
	}
	if asset == machine {
		return 3
	}
	switch machine {
	case "amd64":
		if asset == "386" {
			return 1
		}
	case "arm64":
		// Windows 11 on Arm runs x64 and x86 programs.
		if asset == "amd64" {
			return 1
		}
		if asset == "386" {
			return 0
		}
	}
	return -1
}

func extScore(name string) int {
	switch {
	case strings.HasSuffix(strings.ToLower(name), ".msi"):
		return 2
	case strings.HasSuffix(strings.ToLower(name), ".exe"):
		return 1
	}
	return 0
}

// ErrNoInstaller reports a release without a Windows installer to use.
var ErrNoInstaller = errors.New("这个 release 没有可用的 .msi 或 .exe 安装包")

// AmbiguousError reports installers AppShelf cannot choose between.
type AmbiguousError struct{ Names []string }

func (e *AmbiguousError) Error() string {
	return "无法确定安装包，请填写安装包匹配规则：" + strings.Join(e.Names, "、")
}

// ChooseAsset picks the installer of a release for a machine of the
// architecture: .msi and .exe files, those pattern matches when it is not
// empty, leaving out debug builds; .msi before .exe, then the
// architecture. When the best two rank the same, it chooses neither.
func ChooseAsset(assets []Asset, pattern, machine string) (Asset, error) {
	var re *regexp.Regexp
	if pattern != "" {
		var err error
		if re, err = regexp.Compile("(?i)" + pattern); err != nil {
			return Asset{}, fmt.Errorf("安装包匹配规则无效：%w", err)
		}
	}
	type cand struct {
		a          Asset
		ext, archS int
	}
	var cands []cand
	for _, a := range assets {
		ext := extScore(a.Name)
		if ext == 0 || noise.MatchString(strings.ToLower(a.Name)) {
			continue
		}
		if re != nil && !re.MatchString(a.Name) {
			continue
		}
		s := archScore(archOf(a.Name), machine)
		if s < 0 {
			continue
		}
		cands = append(cands, cand{a, ext, s})
	}
	if len(cands) == 0 {
		if re != nil {
			return Asset{}, errors.New("没有安装包符合匹配规则")
		}
		return Asset{}, ErrNoInstaller
	}
	sort.SliceStable(cands, func(i, j int) bool {
		if cands[i].ext != cands[j].ext {
			return cands[i].ext > cands[j].ext
		}
		return cands[i].archS > cands[j].archS
	})
	if len(cands) > 1 && cands[0].ext == cands[1].ext && cands[0].archS == cands[1].archS {
		var names []string
		for _, c := range cands {
			if c.ext == cands[0].ext && c.archS == cands[0].archS {
				names = append(names, c.a.Name)
			}
		}
		return Asset{}, &AmbiguousError{Names: names}
	}
	return cands[0].a, nil
}
