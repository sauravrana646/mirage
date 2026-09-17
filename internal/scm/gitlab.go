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
	"net/url"
	"strings"
)

type gitLabClient struct {
	cfg    Config
	api    string
	client *http.Client
}

func newGitLab(cfg Config) (*gitLabClient, error) {
	api := cfg.BaseURL
	if api == "" {
		api = "https://gitlab.com/api/v4"
	} else if !strings.Contains(api, "/api/") {
		api = strings.TrimRight(api, "/") + "/api/v4"
	}
	if cfg.ProjectID == "" {
		return nil, fmt.Errorf("gitlab requires projectID (numeric id or group/project path)")
	}
	return &gitLabClient{cfg: cfg, api: strings.TrimRight(api, "/"), client: cfg.HTTPClient}, nil
}

func (g *gitLabClient) projectPath() string {
	return url.PathEscape(g.cfg.ProjectID)
}

func (g *gitLabClient) PostCommitStatus(ctx context.Context, ref Ref, status CommitStatus) error {
	if ref.CommitSHA == "" {
		return fmt.Errorf("commit SHA required")
	}
	form := url.Values{}
	form.Set("state", mapGitLabState(status.State))
	form.Set("name", defaultContext(status.Context))
	form.Set("description", truncate(status.Description, 255))
	if status.TargetURL != "" {
		form.Set("target_url", status.TargetURL)
	}
	path := fmt.Sprintf("/projects/%s/statuses/%s", g.projectPath(), url.PathEscape(ref.CommitSHA))
	return g.doForm(ctx, http.MethodPost, path, form)
}

func (g *gitLabClient) UpsertComment(ctx context.Context, ref Ref, comment Comment) error {
	if ref.PullRequest <= 0 {
		return fmt.Errorf("merge request IID required")
	}
	marker := comment.Marker
	if marker == "" {
		marker = defaultCommentMarker
	}
	listPath := fmt.Sprintf("/projects/%s/merge_requests/%d/notes?per_page=100", g.projectPath(), ref.PullRequest)
	var notes []struct {
		ID   int64  `json:"id"`
		Body string `json:"body"`
	}
	if err := g.doJSON(ctx, http.MethodGet, listPath, nil, &notes); err != nil {
		return err
	}
	var existing int64
	for _, n := range notes {
		if strings.Contains(n.Body, marker) {
			existing = n.ID
			break
		}
	}
	payload := map[string]string{"body": comment.Body}
	if existing > 0 {
		path := fmt.Sprintf("/projects/%s/merge_requests/%d/notes/%d", g.projectPath(), ref.PullRequest, existing)
		return g.doJSON(ctx, http.MethodPut, path, payload, nil)
	}
	path := fmt.Sprintf("/projects/%s/merge_requests/%d/notes", g.projectPath(), ref.PullRequest)
	return g.doJSON(ctx, http.MethodPost, path, payload, nil)
}

func (g *gitLabClient) doJSON(ctx context.Context, method, path string, in, out any) error {
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
	g.auth(req)
	if in != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	return g.finish(req, out)
}

func (g *gitLabClient) doForm(ctx context.Context, method, path string, form url.Values) error {
	req, err := http.NewRequestWithContext(ctx, method, g.api+path, strings.NewReader(form.Encode()))
	if err != nil {
		return err
	}
	g.auth(req)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	return g.finish(req, nil)
}

func (g *gitLabClient) auth(req *http.Request) {
	// CI_JOB_TOKEN uses JOB-TOKEN; PATs / OIDC bearers use PRIVATE-TOKEN or Authorization.
	if strings.HasPrefix(g.cfg.Token, "glcbt-") || g.cfg.Token == "" {
		req.Header.Set("JOB-TOKEN", g.cfg.Token)
	}
	req.Header.Set("PRIVATE-TOKEN", g.cfg.Token)
	req.Header.Set("Authorization", "Bearer "+g.cfg.Token)
}

func (g *gitLabClient) finish(req *http.Request, out any) error {
	resp, err := g.client.Do(req)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode >= 300 {
		return fmt.Errorf("gitlab %s %s: %s: %s", req.Method, req.URL.Path, resp.Status, truncate(string(body), 400))
	}
	if out != nil && len(body) > 0 {
		return json.Unmarshal(body, out)
	}
	return nil
}

func mapGitLabState(s State) string {
	switch s {
	case StateSuccess:
		return "success"
	case StateFailure, StateError:
		return "failed"
	default:
		return "pending"
	}
}
