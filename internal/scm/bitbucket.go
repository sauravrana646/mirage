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

type bitbucketClient struct {
	cfg    Config
	api    string
	client *http.Client
}

func newBitbucket(cfg Config) (*bitbucketClient, error) {
	api := cfg.BaseURL
	if api == "" {
		api = "https://api.bitbucket.org/2.0"
	}
	if cfg.Workspace == "" || cfg.Repo == "" {
		return nil, fmt.Errorf("bitbucket requires workspace and repo slug")
	}
	return &bitbucketClient{cfg: cfg, api: strings.TrimRight(api, "/"), client: cfg.HTTPClient}, nil
}

func (b *bitbucketClient) PostCommitStatus(ctx context.Context, ref Ref, status CommitStatus) error {
	if ref.CommitSHA == "" {
		return fmt.Errorf("commit SHA required")
	}
	key := defaultContext(status.Context)
	payload := map[string]any{
		"state": mapBitbucketState(status.State),
		"key":   key,
		"name":  key,
	}
	if status.Description != "" {
		payload["description"] = truncate(status.Description, 255)
	}
	if status.TargetURL != "" {
		payload["url"] = status.TargetURL
	}
	path := fmt.Sprintf("/repositories/%s/%s/commit/%s/statuses/build", b.cfg.Workspace, b.cfg.Repo, ref.CommitSHA)
	return b.doJSON(ctx, http.MethodPost, path, payload, nil)
}

func (b *bitbucketClient) UpsertComment(ctx context.Context, ref Ref, comment Comment) error {
	if ref.PullRequest <= 0 {
		return fmt.Errorf("pull request id required")
	}
	marker := comment.Marker
	if marker == "" {
		marker = defaultCommentMarker
	}
	listPath := fmt.Sprintf("/repositories/%s/%s/pullrequests/%d/comments", b.cfg.Workspace, b.cfg.Repo, ref.PullRequest)
	var page struct {
		Values []struct {
			ID      int64 `json:"id"`
			Content struct {
				Raw string `json:"raw"`
			} `json:"content"`
		} `json:"values"`
	}
	if err := b.doJSON(ctx, http.MethodGet, listPath, nil, &page); err != nil {
		return err
	}
	var existing int64
	for _, c := range page.Values {
		if strings.Contains(c.Content.Raw, marker) {
			existing = c.ID
			break
		}
	}
	payload := map[string]any{
		"content": map[string]string{"raw": comment.Body},
	}
	if existing > 0 {
		path := fmt.Sprintf("/repositories/%s/%s/pullrequests/%d/comments/%d", b.cfg.Workspace, b.cfg.Repo, ref.PullRequest, existing)
		return b.doJSON(ctx, http.MethodPut, path, payload, nil)
	}
	return b.doJSON(ctx, http.MethodPost, listPath, payload, nil)
}

func (b *bitbucketClient) doJSON(ctx context.Context, method, path string, in, out any) error {
	var rdr io.Reader
	if in != nil {
		raw, err := json.Marshal(in)
		if err != nil {
			return err
		}
		rdr = bytes.NewReader(raw)
	}
	req, err := http.NewRequestWithContext(ctx, method, b.api+path, rdr)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+b.cfg.Token)
	req.Header.Set("Accept", "application/json")
	if in != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := b.client.Do(req)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode >= 300 {
		return fmt.Errorf("bitbucket %s %s: %s: %s", method, path, resp.Status, truncate(string(body), 400))
	}
	if out != nil && len(body) > 0 {
		return json.Unmarshal(body, out)
	}
	return nil
}

func mapBitbucketState(s State) string {
	switch s {
	case StateSuccess:
		return "SUCCESSFUL"
	case StateFailure:
		return "FAILED"
	case StateError:
		return "STOPPED"
	default:
		return "INPROGRESS"
	}
}
