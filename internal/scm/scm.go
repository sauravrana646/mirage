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

// Package scm posts preview status and comments to GitHub, GitLab, and Bitbucket.
// It is used from CI (mirage-notify), never from the reconcile hot path.
package scm

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"
)

// Provider identifies an SCM system.
type Provider string

const (
	ProviderGitHub    Provider = "github"
	ProviderGitLab    Provider = "gitlab"
	ProviderBitbucket Provider = "bitbucket"
)

// State is a commit/build status state.
type State string

const (
	StatePending State = "pending"
	StateSuccess State = "success"
	StateFailure State = "failure"
	StateError   State = "error"
)

const defaultCommentMarker = "<!-- mirage-preview -->"

// AuthMode selects how API credentials are obtained.
type AuthMode string

const (
	// AuthToken uses a static token / PAT / App token / CI job token.
	AuthToken AuthMode = "token"
	// AuthOIDC reads a bearer token from an OIDC/SSO-brokered file or env
	// (e.g. MIRAGE_OIDC_TOKEN / MIRAGE_OIDC_TOKEN_FILE). SCM APIs still expect
	// a bearer credential; OIDC is commonly used to mint short-lived tokens
	// via an identity broker or cloud IAM.
	AuthOIDC AuthMode = "oidc"
)

// Config configures an SCM client.
type Config struct {
	Provider Provider
	Auth     AuthMode
	// Token is the API bearer / basic password material.
	Token string
	// BaseURL overrides the public API root (self-hosted GitLab/GHE/Bitbucket Server).
	BaseURL string
	// Owner is org/user (GitHub), or unused when ProjectID is set (GitLab).
	Owner string
	// Repo is the repository name (GitHub) or slug (Bitbucket).
	Repo string
	// ProjectID is GitLab numeric ID or "group/project" path.
	ProjectID string
	// Workspace is Bitbucket Cloud workspace.
	Workspace string
	// HTTPClient optional; defaults to http.DefaultClient with timeout.
	HTTPClient *http.Client
}

// Ref identifies the commit and optional PR/MR.
type Ref struct {
	CommitSHA   string
	PullRequest int
}

// CommitStatus is reported on the commit (and surfaced on the PR/MR).
type CommitStatus struct {
	State       State
	Context     string
	Description string
	TargetURL   string
}

// Comment is an upsertable PR/MR comment body.
type Comment struct {
	Marker string // unique HTML/markdown marker for idempotent upsert
	Body   string
}

// Client posts statuses and comments.
type Client interface {
	PostCommitStatus(ctx context.Context, ref Ref, status CommitStatus) error
	UpsertComment(ctx context.Context, ref Ref, comment Comment) error
}

// New creates a Client for the configured provider.
func New(cfg Config) (Client, error) {
	if cfg.HTTPClient == nil {
		cfg.HTTPClient = &http.Client{Timeout: 30 * time.Second}
	}
	token, err := resolveToken(cfg)
	if err != nil {
		return nil, err
	}
	cfg.Token = token
	switch Provider(strings.ToLower(string(cfg.Provider))) {
	case ProviderGitHub, "":
		cfg.Provider = ProviderGitHub
		return newGitHub(cfg)
	case ProviderGitLab:
		return newGitLab(cfg)
	case ProviderBitbucket:
		return newBitbucket(cfg)
	default:
		return nil, fmt.Errorf("unsupported SCM provider %q (want github|gitlab|bitbucket)", cfg.Provider)
	}
}

// ConfigFromEnv builds Config from MIRAGE_SCM_* and common CI variables.
func ConfigFromEnv() (Config, error) {
	provider := strings.ToLower(strings.TrimSpace(os.Getenv("MIRAGE_SCM_PROVIDER")))
	if provider == "" {
		switch {
		case os.Getenv("GITHUB_ACTIONS") == "true":
			provider = string(ProviderGitHub)
		case os.Getenv("GITLAB_CI") == "true":
			provider = string(ProviderGitLab)
		case os.Getenv("BITBUCKET_BUILD_NUMBER") != "":
			provider = string(ProviderBitbucket)
		default:
			provider = string(ProviderGitHub)
		}
	}

	auth := AuthMode(strings.ToLower(firstNonEmpty(os.Getenv("MIRAGE_SCM_AUTH"), string(AuthToken))))
	cfg := Config{
		Provider:  Provider(provider),
		Auth:      auth,
		BaseURL:   strings.TrimRight(os.Getenv("MIRAGE_SCM_BASE_URL"), "/"),
		Owner:     firstNonEmpty(os.Getenv("MIRAGE_SCM_OWNER"), os.Getenv("GITHUB_REPOSITORY_OWNER")),
		Repo:      firstNonEmpty(os.Getenv("MIRAGE_SCM_REPO"), bitbucketRepoSlug(), githubRepoName()),
		ProjectID: firstNonEmpty(os.Getenv("MIRAGE_SCM_PROJECT_ID"), os.Getenv("CI_PROJECT_ID"), os.Getenv("CI_PROJECT_PATH")),
		Workspace: firstNonEmpty(os.Getenv("MIRAGE_SCM_WORKSPACE"), os.Getenv("BITBUCKET_WORKSPACE")),
	}
	return cfg, nil
}

func resolveToken(cfg Config) (string, error) {
	switch cfg.Auth {
	case AuthOIDC, "sso":
		tok := firstNonEmpty(
			cfg.Token,
			os.Getenv("MIRAGE_OIDC_TOKEN"),
			os.Getenv("MIRAGE_SCM_TOKEN"),
		)
		if tok == "" {
			if path := os.Getenv("MIRAGE_OIDC_TOKEN_FILE"); path != "" {
				b, err := os.ReadFile(path)
				if err != nil {
					return "", fmt.Errorf("read OIDC token file: %w", err)
				}
				tok = strings.TrimSpace(string(b))
			}
		}
		if tok == "" {
			return "", fmt.Errorf("auth=oidc requires MIRAGE_OIDC_TOKEN, MIRAGE_OIDC_TOKEN_FILE, or MIRAGE_SCM_TOKEN (SSO-brokered bearer)")
		}
		return tok, nil
	default:
		tok := firstNonEmpty(
			cfg.Token,
			os.Getenv("MIRAGE_SCM_TOKEN"),
			os.Getenv("GITHUB_TOKEN"),
			os.Getenv("GH_TOKEN"),
			os.Getenv("GITLAB_TOKEN"),
			os.Getenv("CI_JOB_TOKEN"),
			os.Getenv("BITBUCKET_TOKEN"),
			os.Getenv("BITBUCKET_ACCESS_TOKEN"),
		)
		if tok == "" {
			return "", fmt.Errorf("SCM token required (MIRAGE_SCM_TOKEN or provider CI token)")
		}
		return tok, nil
	}
}

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if strings.TrimSpace(v) != "" {
			return strings.TrimSpace(v)
		}
	}
	return ""
}

func githubRepoName() string {
	full := os.Getenv("GITHUB_REPOSITORY") // owner/repo
	if full == "" {
		return ""
	}
	parts := strings.SplitN(full, "/", 2)
	if len(parts) == 2 {
		return parts[1]
	}
	return full
}

func bitbucketRepoSlug() string {
	return firstNonEmpty(os.Getenv("BITBUCKET_REPO_SLUG"), os.Getenv("MIRAGE_SCM_REPO"))
}

// PreviewCommentBody builds a standard Mirage preview markdown comment.
func PreviewCommentBody(marker, phase, url, image, namespace string) string {
	if marker == "" {
		marker = defaultCommentMarker
	}
	if phase == "Ready" || phase == "success" {
		if url == "" {
			url = "_(ingress not configured)_"
		}
		return fmt.Sprintf("%s\n### Mirage preview\n\n| | |\n|---|---|\n| **Status** | Ready |\n| **URL** | %s |\n| **Image** | `%s` |\n| **Namespace** | `%s` |\n",
			marker, url, image, namespace)
	}
	return fmt.Sprintf("%s\n### Mirage preview\n\nPreview did not become Ready (phase: `%s`).\n\nImage: `%s`\nNamespace: `%s`\n",
		marker, phase, image, namespace)
}
