package pr

import (
	"encoding/json"
	"fmt"
)

// PRDetails holds the PR metadata fetched from GitHub API.
type PRDetails struct {
	Title        string
	Description  string
	Author       string
	BaseBranch   string
	HeadBranch   string
	BaseSHA      string
	HeadSHA      string
	State        string
	URL          string
	CloneURL     string // head.repo.clone_url — used to clone the PR's source repository
	HeadRepoName string // head.repo.full_name — "owner/repo" for the head
	BaseRepoName string // base.repo.full_name — "owner/repo" for the base
}

// UnmarshalJSON handles the nested GitHub API response structure.
func (d *PRDetails) UnmarshalJSON(data []byte) error {
	var raw struct {
		Title   string `json:"title"`
		Body    string `json:"body"`
		State   string `json:"state"`
		HTMLURL string `json:"html_url"`
		User    struct {
			Login string `json:"login"`
		} `json:"user"`
		Base struct {
			Ref  string `json:"ref"`
			SHA  string `json:"sha"`
			Repo struct {
				FullName string `json:"full_name"`
			} `json:"repo"`
		} `json:"base"`
		Head struct {
			Ref  string `json:"ref"`
			SHA  string `json:"sha"`
			Repo struct {
				CloneURL string `json:"clone_url"`
				FullName string `json:"full_name"`
			} `json:"repo"`
		} `json:"head"`
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	d.Title = raw.Title
	d.Description = raw.Body
	d.Author = raw.User.Login
	d.BaseBranch = raw.Base.Ref
	d.HeadBranch = raw.Head.Ref
	d.BaseSHA = raw.Base.SHA
	d.HeadSHA = raw.Head.SHA
	d.State = raw.State
	d.URL = raw.HTMLURL
	d.CloneURL = raw.Head.Repo.CloneURL
	d.HeadRepoName = raw.Head.Repo.FullName
	d.BaseRepoName = raw.Base.Repo.FullName
	return nil
}

// ChangedFile represents a file changed in the PR.
type ChangedFile struct {
	Filename         string `json:"filename"`
	Status           string `json:"status"`
	Additions        int    `json:"additions"`
	Deletions        int    `json:"deletions"`
	Changes          int    `json:"changes"`
	Patch            string `json:"patch"`
	PreviousFilename string `json:"previous_filename"`
}

// PRData aggregates all fetched PR information.
type PRData struct {
	Info    *PRInfo
	Details *PRDetails
	Files   []ChangedFile
}

// APIError wraps GitHub API errors with status information.
type APIError struct {
	StatusCode int
	Message    string
	URL        string
}

func (e *APIError) Error() string {
	return fmt.Sprintf("GitHub API error (%d): %s", e.StatusCode, e.Message)
}
