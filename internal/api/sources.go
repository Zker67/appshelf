package api

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"appshelf/internal/catalog"
)

// Sources are winget, scoop and GitHub: whether they can be used here,
// setting up winget and scoop, and finding a package's exact identity.
type Sources struct{ a *App }

// Overview returns the sources' state and GitHub's quotas.
func (s *Sources) Overview() SourcesOverview {
	a := s.a
	out := SourcesOverview{Sources: []SourceInfo{}}
	a.read(func() {
		c := a.core
		for _, t := range c.SourceTypes() {
			info := SourceInfo{Type: t}
			if av, ok := c.Availability(t); ok {
				info.Checked, info.OK, info.Reason = true, av.OK, av.Reason
				info.Fixable = !av.OK && av.Fixable && c.CanFix(t)
				if av.OK && av.Helper != "" && c.CanFix(t) {
					info.Helper = av.Helper
				}
			}
			out.Sources = append(out.Sources, info)
		}
		rate, token := c.GitHubRate()
		out.GitHubRate, out.GitHubToken = rateOf(rate), token
	})
	a.mu.Lock()
	out.SetupSeen = a.settings.SourceSetupSeen
	out.GitHubTokenFrom = a.tokenFrom
	a.mu.Unlock()
	if g := a.cfg.Sources.GitHub; g != nil {
		out.GitHubSearchRate = rateOf(g.SearchRate())
	}
	return out
}

// Fix makes a source usable for the user: for winget it installs the
// PowerShell module, for scoop scoop itself and aria2, or aria2 alone when
// it is what scoop misses. It then checks the sources and
// reads the states again. It takes minutes.
func (s *Sources) Fix(sourceType catalog.SourceType) error {
	a := s.a
	<-a.ready
	done := make(chan error, 1)
	a.write([]ChangeKind{ChangedSources}, func() {
		if !a.core.CanFix(sourceType) {
			done <- errors.New("这个来源无法由 AppShelf 安装")
			return
		}
		a.core.Fix(sourceType, func(err error) { done <- err })
	})
	err := <-done
	a.note.mark(ChangedSources, ChangedShelf)
	if err == nil {
		a.inv.refresh(0)
	}
	return err
}

// DismissSetup keeps that the first prompt to set up the sources was
// answered, so that it is not shown again.
func (s *Sources) DismissSetup() error {
	a := s.a
	<-a.ready
	a.mu.Lock()
	a.settings.SourceSetupSeen = true
	st := a.settings
	a.mu.Unlock()
	if err := saveSettings(a.cfg.Settings, st); err != nil {
		return fmt.Errorf("保存设置失败：%w", err)
	}
	a.note.mark(ChangedSources)
	return nil
}

// Search finds packages of winget or scoop, for an app's source.
func (s *Sources) Search(ctx context.Context, sourceType catalog.SourceType, query string) ([]SearchHit, error) {
	a := s.a
	<-a.ready
	var sr *searcher
	for _, one := range a.searchers() {
		if one.t == sourceType {
			sr = &one
		}
	}
	if sr == nil {
		return nil, errors.New("这个来源不支持搜索")
	}
	if av := a.availability(ctx, sourceType, sr.check); !av.OK {
		return nil, errors.New(av.Reason)
	}
	if strings.TrimSpace(query) == "" {
		return []SearchHit{}, nil
	}
	var versions <-chan map[string]string
	if sr.versions != nil {
		versions = a.found.versions(ctx, sourceType, query, sr.versions)
	}
	res, err := a.found.results(ctx, sourceType, query, sr.search)
	if err != nil {
		return nil, err
	}
	if versions != nil && lacksVersion(res) {
		select {
		case v := <-versions:
			res, _ = withVersions(res, v)
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
	return a.hits(sourceType, query, res), nil
}

// CheckGitHub reads the latest release of a repository and the installer
// AppShelf would choose from it with the pattern.
func (s *Sources) CheckGitHub(ctx context.Context, repo, assetPattern string) (GitHubCheck, error) {
	a := s.a
	<-a.ready
	g := a.cfg.Sources.GitHub
	if g == nil {
		return GitHubCheck{}, errors.New("GitHub 来源不可用")
	}
	app := catalog.App{Name: repo, Source: catalog.Source{Type: catalog.GitHub, Repo: strings.Trim(strings.TrimSpace(repo), "/"), AssetPattern: strings.TrimSpace(assetPattern)}, Detect: &catalog.Detect{ExePath: "x"}}
	if err := catalog.ValidateApp(app); err != nil {
		return GitHubCheck{}, err
	}
	rel, asset, err := g.Describe(ctx, app)
	a.note.mark(ChangedSources)
	if err != nil {
		return GitHubCheck{}, err
	}
	return GitHubCheck{Tag: rel.Tag, Asset: asset.Name}, nil
}
