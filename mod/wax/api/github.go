package api

import (
	"context"

	"github.com/google/go-github/v73/github"
)

type Issue = github.Issue

type status string

const Open status = "open"
const Closed status = "closed"
const All status = "all"

type Github struct {
	client *github.Client
}

func New(token string) *Github {
	return &Github{
		client: github.NewClient(nil).WithAuthToken(token),
	}
}

func (gh *Github) Issues(ctx context.Context, state status, owner, repo string) ([]*Issue, error) {
	opts := &github.IssueListByRepoOptions{
		ListOptions: github.ListOptions{PerPage: 100},
		State:       string(state),
	}

	var allIssues []*Issue
	for {
		issues, response, err := gh.client.Issues.ListByRepo(ctx, owner, repo, opts)
		if err != nil {
			return nil, err
		}
		allIssues = append(allIssues, issues...)
		if response.NextPage == 0 {
			break
		}
		opts.ListOptions.Page = response.NextPage
	}

	return allIssues, nil
}
