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
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestGitHubStatusAndComment(t *testing.T) {
	var sawStatus, sawComment bool
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/repos/org/app/statuses/abc123":
			sawStatus = true
			w.WriteHeader(http.StatusCreated)
			_, _ = w.Write([]byte(`{"id":1}`))
		case r.Method == http.MethodGet && r.URL.Path == "/repos/org/app/issues/7/comments":
			_, _ = w.Write([]byte(`[]`))
		case r.Method == http.MethodPost && r.URL.Path == "/repos/org/app/issues/7/comments":
			sawComment = true
			w.WriteHeader(http.StatusCreated)
			_, _ = w.Write([]byte(`{"id":2}`))
		default:
			t.Fatalf("unexpected %s %s", r.Method, r.URL.Path)
		}
	}))
	defer srv.Close()

	c, err := New(Config{
		Provider: ProviderGitHub, Token: "t", Owner: "org", Repo: "app",
		BaseURL: srv.URL, HTTPClient: srv.Client(),
	})
	if err != nil {
		t.Fatal(err)
	}
	ref := Ref{CommitSHA: "abc123", PullRequest: 7}
	if err := c.PostCommitStatus(context.Background(), ref, CommitStatus{State: StatePending, Description: "deploying"}); err != nil {
		t.Fatal(err)
	}
	body := PreviewCommentBody("<!-- mirage-preview -->", "Ready", "https://pr.example.com", "img@sha", "preview-pr-7")
	if err := c.UpsertComment(context.Background(), ref, Comment{Marker: "<!-- mirage-preview -->", Body: body}); err != nil {
		t.Fatal(err)
	}
	if !sawStatus || !sawComment {
		t.Fatalf("sawStatus=%v sawComment=%v", sawStatus, sawComment)
	}
}

func TestGitLabStatus(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Fatalf("unexpected method %s", r.Method)
		}
		// httptest may decode %2F in Path; accept either form.
		if !strings.Contains(r.URL.Path, "/projects/") || !strings.HasSuffix(r.URL.Path, "/statuses/deadbeef") {
			t.Fatalf("unexpected %s %s", r.Method, r.URL.Path)
		}
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{"id":1}`))
	}))
	defer srv.Close()
	c, err := New(Config{
		Provider: ProviderGitLab, Token: "glpat-x", ProjectID: "group/proj",
		BaseURL: srv.URL + "/api/v4", HTTPClient: srv.Client(),
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := c.PostCommitStatus(context.Background(), Ref{CommitSHA: "deadbeef"}, CommitStatus{State: StateSuccess, TargetURL: "https://x"}); err != nil {
		t.Fatal(err)
	}
}

func TestBitbucketStatus(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		want := "/repositories/ws/repo/commit/cafe/statuses/build"
		if r.Method != http.MethodPost || r.URL.Path != want {
			t.Fatalf("unexpected %s %s", r.Method, r.URL.Path)
		}
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{"key":"mirage/preview"}`))
	}))
	defer srv.Close()
	c, err := New(Config{
		Provider: ProviderBitbucket, Token: "bb", Workspace: "ws", Repo: "repo",
		BaseURL: srv.URL, HTTPClient: srv.Client(),
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := c.PostCommitStatus(context.Background(), Ref{CommitSHA: "cafe"}, CommitStatus{State: StateFailure}); err != nil {
		t.Fatal(err)
	}
}

func TestUnsupportedProvider(t *testing.T) {
	_, err := New(Config{Provider: "gitea", Token: "x", Owner: "a", Repo: "b"})
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestPreviewCommentBody(t *testing.T) {
	s := PreviewCommentBody("", "Ready", "https://u", "img", "ns")
	for _, part := range []string{"Ready", "https://u", "img", "ns", "<!-- mirage-preview -->"} {
		if !strings.Contains(s, part) {
			t.Fatalf("missing %q in %s", part, s)
		}
	}
	built := PreviewCommentBody("", "Built", "", "img@sha", "preview-pr-2")
	if strings.Contains(built, "did not become Ready") {
		t.Fatalf("Built phase should not look like a failure: %s", built)
	}
	if !strings.Contains(built, "cluster apply skipped") {
		t.Fatalf("expected skip messaging: %s", built)
	}
}
