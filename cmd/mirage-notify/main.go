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

// mirage-notify posts preview commit statuses and PR/MR comments to
// GitHub, GitLab, or Bitbucket. Used from CI — not from the reconcile loop.
package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/sauravrana646/mirage/internal/scm"
)

func main() {
	if len(os.Args) < 2 {
		usage()
		os.Exit(2)
	}
	switch os.Args[1] {
	case "status":
		os.Exit(runStatus(os.Args[2:]))
	case "comment":
		os.Exit(runComment(os.Args[2:]))
	case "help", "-h", "--help":
		usage()
	default:
		fmt.Fprintf(os.Stderr, "unknown command %q\n", os.Args[1])
		usage()
		os.Exit(2)
	}
}

func usage() {
	fmt.Fprintf(os.Stderr, `mirage-notify — report Mirage preview status to GitHub / GitLab / Bitbucket

Usage:
  mirage-notify status  --state pending|success|failure|error [flags]
  mirage-notify comment --phase Ready|Failed [...] [flags]

Environment (see docs/scm-integration.md):
  MIRAGE_SCM_PROVIDER   github|gitlab|bitbucket
  MIRAGE_SCM_AUTH       token|oidc  (default token)
  MIRAGE_SCM_TOKEN      API token (or GITHUB_TOKEN / GITLAB_TOKEN / CI_JOB_TOKEN / BITBUCKET_TOKEN)
  MIRAGE_OIDC_TOKEN(_FILE)  SSO/OIDC-brokered bearer when auth=oidc
  MIRAGE_SCM_BASE_URL   self-hosted API root
  MIRAGE_SCM_OWNER / MIRAGE_SCM_REPO / MIRAGE_SCM_PROJECT_ID / MIRAGE_SCM_WORKSPACE
`)
}

func runStatus(args []string) int {
	fs := flag.NewFlagSet("status", flag.ContinueOnError)
	state := fs.String("state", "pending", "pending|success|failure|error")
	contextName := fs.String("context", "mirage/preview", "status context/name/key")
	desc := fs.String("description", "", "short description")
	target := fs.String("target-url", "", "link shown in the SCM UI")
	sha := fs.String("sha", env("MIRAGE_SCM_SHA", "GITHUB_SHA", "CI_COMMIT_SHA", "BITBUCKET_COMMIT"), "commit SHA")
	_ = fs.Int("pr", envInt("MIRAGE_SCM_PR", "CI_MERGE_REQUEST_IID", "BITBUCKET_PR_ID"), "PR/MR number")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	client, err := clientFromEnv()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	err = client.PostCommitStatus(context.Background(), scm.Ref{CommitSHA: *sha}, scm.CommitStatus{
		State: scm.State(strings.ToLower(*state)), Context: *contextName, Description: *desc, TargetURL: *target,
	})
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	return 0
}

func runComment(args []string) int {
	fs := flag.NewFlagSet("comment", flag.ContinueOnError)
	phase := fs.String("phase", "Pending", "Ready|Failed|…")
	url := fs.String("url", "", "preview URL")
	image := fs.String("image", "", "image digest ref")
	namespace := fs.String("namespace", "", "target namespace")
	marker := fs.String("marker", "<!-- mirage-preview -->", "idempotent comment marker")
	sha := fs.String("sha", env("MIRAGE_SCM_SHA", "GITHUB_SHA", "CI_COMMIT_SHA", "BITBUCKET_COMMIT"), "commit SHA")
	pr := fs.Int("pr", envInt("MIRAGE_SCM_PR", "CI_MERGE_REQUEST_IID", "BITBUCKET_PR_ID"), "PR/MR number")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if *pr <= 0 {
		// GitHub Actions often passes via env
		if n := envInt("GITHUB_PR_NUMBER"); n > 0 {
			*pr = n
		}
	}
	if *pr <= 0 {
		fmt.Fprintln(os.Stderr, "--pr / MIRAGE_SCM_PR required for comments")
		return 2
	}
	client, err := clientFromEnv()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	body := scm.PreviewCommentBody(*marker, *phase, *url, *image, *namespace)
	err = client.UpsertComment(context.Background(), scm.Ref{CommitSHA: *sha, PullRequest: *pr}, scm.Comment{
		Marker: *marker, Body: body,
	})
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	return 0
}

func clientFromEnv() (scm.Client, error) {
	cfg, err := scm.ConfigFromEnv()
	if err != nil {
		return nil, err
	}
	// Fill GitHub owner/repo from GITHUB_REPOSITORY when needed.
	if cfg.Provider == scm.ProviderGitHub && (cfg.Owner == "" || cfg.Repo == "") {
		if full := os.Getenv("GITHUB_REPOSITORY"); full != "" {
			o, r := scm.ParseGitHubRepo(full)
			if cfg.Owner == "" {
				cfg.Owner = o
			}
			if cfg.Repo == "" {
				cfg.Repo = r
			}
		}
	}
	return scm.New(cfg)
}

func env(keys ...string) string {
	for _, k := range keys {
		if v := strings.TrimSpace(os.Getenv(k)); v != "" {
			return v
		}
	}
	return ""
}

func envInt(keys ...string) int {
	v := env(keys...)
	if v == "" {
		return 0
	}
	n, _ := strconv.Atoi(v)
	return n
}
