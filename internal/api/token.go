package api

import (
	"context"
	"errors"
	"fmt"
	"log"
	"strings"

	"appshelf/internal/secret"
)

// loadToken uses the GitHub token saved in the settings, before the one of
// the environment, as AppShelf starts.
func (a *App) loadToken() {
	a.token, a.tokenFrom = a.cfg.EnvToken, TokenNone
	if a.token != "" {
		a.tokenFrom = TokenEnv
	}
	if a.cfg.TokenFile == "" {
		return
	}
	saved, err := secret.Load(a.cfg.TokenFile)
	if err != nil {
		log.Printf("appshelf: the saved GitHub token is not read: %v", err)
		return
	}
	if saved == "" {
		return
	}
	a.token, a.tokenFrom = saved, TokenSaved
	if g := a.cfg.Sources.GitHub; g != nil {
		g.SetToken(saved)
	}
}

// validToken checks the shape of a token, so that what is not one is not
// sent to GitHub: GitHub's tokens are letters, digits and _.
func validToken(token string) error {
	if token == "" {
		return errors.New("请填写 token")
	}
	if len(token) > 255 {
		return errors.New("token 太长，请确认只复制了 token 本身")
	}
	for _, r := range token {
		if !(r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r == '_' || r == '-') {
			return errors.New("token 只能包含字母、数字、下划线和短横线，请确认复制完整、没有多余字符")
		}
	}
	return nil
}

// SetGitHubToken checks a GitHub token with GitHub, which costs no
// request, then keeps it encrypted for this Windows user and uses it at
// once, before GH_TOKEN and GITHUB_TOKEN. A token GitHub refuses is not
// kept. The token never comes back to the page.
func (s *Sources) SetGitHubToken(ctx context.Context, token string) error {
	a := s.a
	<-a.ready
	g := a.cfg.Sources.GitHub
	if g == nil || a.cfg.TokenFile == "" {
		return errors.New("这个版本不能保存 GitHub token")
	}
	token = strings.TrimSpace(token)
	if err := validToken(token); err != nil {
		return err
	}
	if err := g.UseToken(ctx, token); err != nil {
		return err
	}
	if err := secret.Save(a.cfg.TokenFile, token); err != nil {
		a.mu.Lock()
		prev := a.token
		a.mu.Unlock()
		g.SetToken(prev)
		return fmt.Errorf("没有保存 token：%w", err)
	}
	a.mu.Lock()
	a.token, a.tokenFrom = token, TokenSaved
	a.mu.Unlock()
	a.tokenChanged()
	return nil
}

// ClearGitHubToken forgets the token saved in the settings; GH_TOKEN or
// GITHUB_TOKEN is used again, when set.
func (s *Sources) ClearGitHubToken() error {
	a := s.a
	<-a.ready
	if a.cfg.TokenFile == "" {
		return nil
	}
	if err := secret.Delete(a.cfg.TokenFile); err != nil {
		return fmt.Errorf("无法删除保存的 token：%w", err)
	}
	from := TokenNone
	if a.cfg.EnvToken != "" {
		from = TokenEnv
	}
	a.mu.Lock()
	a.token, a.tokenFrom = a.cfg.EnvToken, from
	a.mu.Unlock()
	if g := a.cfg.Sources.GitHub; g != nil {
		g.SetToken(a.cfg.EnvToken)
	}
	a.tokenChanged()
	return nil
}

// tokenChanged reads the states again: GitHub apps that ran out of quota
// can be read with the new token.
func (a *App) tokenChanged() {
	a.write([]ChangeKind{ChangedSources, ChangedShelf}, func() { a.core.Refresh() })
}
