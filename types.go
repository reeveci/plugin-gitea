package main

import "github.com/reeveci/reeve-lib/schema"

type Webhook struct {
	Ref string `json:"ref"`

	HeadCommit struct {
		ID      string `json:"id"`
		Message string `json:"message"`
		ModifiedFiles
	} `json:"head_commit"`

	Commits []ModifiedFiles `json:"commits"`

	Repository struct {
		FullName      string `json:"full_name"`
		HtmlURL       string `json:"html_url"`
		CloneURL      string `json:"clone_url"`
		DefaultBranch string `json:"default_branch"`
	} `json:"repository"`
}

type ModifiedFiles struct {
	Added    []string `json:"added"`
	Removed  []string `json:"removed"`
	Modified []string `json:"modified"`
}

type Document struct {
	Type                      string `yaml:"type"`
	schema.PipelineDefinition `yaml:",inline"`
	Value                     string `yaml:"value"`
	Cron                      string `yaml:"cron"`
	Action                    string `yaml:"action"`
}

type UserResponse struct {
	ID int `json:"id"`
}

type AssigneesResponse []UserResponse

type SearchResponse struct {
	Data []struct {
		FullName      string `json:"full_name"`
		HtmlURL       string `json:"html_url"`
		CloneURL      string `json:"clone_url"`
		DefaultBranch string `json:"default_branch"`
	} `json:"data"`
}

type CommitResponse struct {
	Commit struct {
		ID string `json:"id"`
	} `json:"commit"`
}
