/*
Copyright 2026 Saurav Rana.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package scm

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
)

type gitHubClient struct {
	cfg    Config
	api    string
	client *http.Client
}

func newGitHub(cfg Config) (*gitHubClient, error) {
	api := cfg.BaseURL
	if api == "" {
		api = "https://api.github.com"
	}
	if cfg.Owner == "" || cfg.Repo == "" {
		return nil, fmt.Errorf("github requires owner and repo")
	}
	return &gitHubClient{cfg: cfg, api: strings.TrimRight(api, "/"), client: cfg.HTTPClient}, nil
}

func (g *gitHubClient) PostCommitStatus(ctx context.Context, ref Ref, status CommitStatus) error {
	if ref.CommitSHA == "" {
		return fmt.Errorf("commit SHA required")
	}
	body := map[string]string{
		"state":       mapGitHubState(status.State),
		"context":     defaultContext(status.Context),
		"description": truncate(status.Description, 140),
	}
	if status.TargetURL != "" {
		body["target_url"] = status.TargetURL
	}
	path := fmt.Sprintf("/repos/%s/%s/statuses/%s", g.cfg.Owner, g.cfg.Repo, ref.CommitSHA)
	return g.doJSON(ctx, http.MethodPost, path, body, nil)
}

func (g *gitHubClient) UpsertComment(ctx context.Context, ref Ref, comment Comment) error {
	if ref.PullRequest <= 0 {
		return fmt.Errorf("pull request number required")
	}
	marker := comment.Marker
	if marker == "" {
		marker = defaultCommentMarker
	}
	listPath := fmt.Sprintf("/repos/%s/%s/issues/%d/comments?per_page=100", g.cfg.Owner, g.cfg.Repo, ref.PullRequest)
	var comments []struct {
		ID   int64  `json:"id"`
		Body string `json:"body"`
	}
	if err := g.doJSON(ctx, http.MethodGet, listPath, nil, &comments); err != nil {
		return err
	}
	var existing int64
	for _, c := range comments {
		if strings.Contains(c.Body, marker) {
			existing = c.ID
			break
		}
	}
	payload := map[string]string{"body": comment.Body}
	if existing > 0 {
		return g.doJSON(ctx, http.MethodPatch, fmt.Sprintf("/repos/%s/%s/issues/comments/%d", g.cfg.Owner, g.cfg.Repo, existing), payload, nil)
	}
	return g.doJSON(ctx, http.MethodPost, fmt.Sprintf("/repos/%s/%s/issues/%d/comments", g.cfg.Owner, g.cfg.Repo, ref.PullRequest), payload, nil)
}

func (g *gitHubClient) doJSON(ctx context.Context, method, path string, in, out any) error {
	var rdr io.Reader
	if in != nil {
		b, err := json.Marshal(in)
		if err != nil {
			return err
		}
		rdr = bytes.NewReader(b)
	}
	req, err := http.NewRequestWithContext(ctx, method, g.api+path, rdr)
	if err != nil {
		return err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("Authorization", "Bearer "+g.cfg.Token)
	req.Header.Set("X-GitHub-Api-Version", "2022-11-28")
	if in != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := g.client.Do(req)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode >= 300 {
		return fmt.Errorf("github %s %s: %s: %s", method, path, resp.Status, truncate(string(body), 400))
	}
	if out != nil && len(body) > 0 {
		return json.Unmarshal(body, out)
	}
	return nil
}

func mapGitHubState(s State) string {
	switch s {
	case StateSuccess:
		return string(StateSuccess)
	case StateFailure:
		return string(StateFailure)
	case StateError:
		return string(StateError)
	default:
		return string(StatePending)
	}
}

func defaultContext(c string) string {
	if c == "" {
		return "mirage/preview"
	}
	return c
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n-1] + "…"
}

// ParseGitHubRepo splits "https://github.com/org/repo" or "org/repo" into owner/repo.
func ParseGitHubRepo(repoURL string) (owner, repo string) {
	repoURL = strings.TrimSuffix(strings.TrimSpace(repoURL), ".git")
	repoURL = strings.TrimPrefix(repoURL, "https://")
	repoURL = strings.TrimPrefix(repoURL, "http://")
	repoURL = strings.TrimPrefix(repoURL, "github.com/")
	parts := strings.Split(repoURL, "/")
	if len(parts) >= 2 {
		return parts[0], parts[1]
	}
	return "", ""
}
