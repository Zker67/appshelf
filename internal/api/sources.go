package api

import (
	"context"
	"errors"
	"strings"

	"appshelf/internal/catalog"
	"appshelf/internal/source"
)

// Sources are winget, scoop and GitHub: whether they can be used here,
// fixing winget, and finding a package's exact identity.
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
				info.Checked, info.OK, info.Reason, info.Fixable = true, av.OK, av.Reason, av.Fixable
			}
			out.Sources = append(out.Sources, info)
		}
		out.CanInstallModule = c.CanInstallModule()
		rate, token := c.GitHubRate()
		out.GitHubRate, out.GitHubToken = rateOf(rate), token
	})
	if g := a.cfg.Sources.GitHub; g != nil {
		out.GitHubSearchRate = rateOf(g.SearchRate())
	}
	return out
}

// InstallModule installs winget's PowerShell module for the user, then
// checks the sources and reads the states again. It takes minutes.
func (s *Sources) InstallModule() error {
	a := s.a
	done := make(chan error, 1)
	a.write([]ChangeKind{ChangedSources}, func() {
		a.core.InstallModule(func(err error) { done <- err })
	})
	err := <-done
	a.note.mark(ChangedSources, ChangedShelf)
	if err == nil {
		a.inv.refresh(0)
	}
	return err
}

// Search finds packages of winget or scoop, for an app's source.
func (s *Sources) Search(ctx context.Context, sourceType catalog.SourceType, query string) ([]SearchHit, error) {
	a := s.a
	<-a.ready
	var search func(context.Context, string) ([]source.SearchResult, error)
	var check func(context.Context) source.Availability
	switch sourceType {
	case catalog.Winget:
		if w := a.cfg.Sources.Winget; w != nil {
			search, check = w.Search, w.Check
		}
	case catalog.Scoop:
		if sc := a.cfg.Sources.Scoop; sc != nil {
			search, check = sc.Search, sc.Check
		}
	}
	if search == nil {
		return nil, errors.New("这个来源不支持搜索")
	}
	if av := a.availability(ctx, sourceType, check); !av.OK {
		return nil, errors.New(av.Reason)
	}
	if strings.TrimSpace(query) == "" {
		return []SearchHit{}, nil
	}
	res, err := search(ctx, query)
	if err != nil {
		return nil, err
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
