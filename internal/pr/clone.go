package pr

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/config"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/transport/http"
)

// DefaultCloneBaseDir returns the default parent directory for cloned PR repos.
// Uses $HOME/.ai-pr-review/pr so that clones survive across project directories
// and act as a shared cache.
func DefaultCloneBaseDir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("cannot find home directory: %w", err)
	}
	return filepath.Join(home, ".ai-pr-review", "pr"), nil
}

// CloneManager handles cloning and managing PR target repositories using go-git.
type CloneManager struct {
	BaseDir string // parent directory for cloned repos
}

// NewCloneManager creates a CloneManager rooted at baseDir.
func NewCloneManager(baseDir string) *CloneManager {
	return &CloneManager{BaseDir: baseDir}
}

// RepoDir returns the expected directory path for a cloned PR repo.
// Includes the PR number so concurrent PRs from the same repo don't conflict.
func (m *CloneManager) RepoDir(owner, repo string, pullNumber int) string {
	return filepath.Join(m.BaseDir, fmt.Sprintf("%s-%s-%d", owner, repo, pullNumber))
}

// ClonePRRepo ensures the PR's head repository is cloned and checked out at
// the PR head ref. Returns the absolute path to the cloned repository.
//
// For same-repo PRs (head and base are the same repository), the branch is
// fetched via refs/heads/<headRef> from origin. For fork PRs (head and base
// differ), the fork's clone URL is added as a second remote.
func (m *CloneManager) ClonePRRepo(ctx context.Context, owner, repo string, pullNumber int, headRef, cloneURL, token string) (string, error) {
	targetPath, err := filepath.Abs(m.RepoDir(owner, repo, pullNumber))
	if err != nil {
		return "", fmt.Errorf("resolve clone path: %w", err)
	}

	if cloneURL == "" {
		cloneURL = fmt.Sprintf("https://github.com/%s/%s.git", owner, repo)
	}

	auth := makeAuth(token)
	headBranch := plumbing.NewBranchReferenceName(headRef)
	headRefSpec := config.RefSpec(fmt.Sprintf("+%s:%s", headBranch, headBranch))

	if err := os.MkdirAll(filepath.Dir(targetPath), 0o755); err != nil {
		return "", fmt.Errorf("create base directory: %w", err)
	}

	var r *git.Repository

	if isGitRepo(targetPath) {
		r, err = git.PlainOpen(targetPath)
		if err != nil {
			return "", fmt.Errorf("open existing repo: %w", err)
		}
		wt, _ := r.Worktree()
		if wt != nil {
			_ = wt.Reset(&git.ResetOptions{Mode: git.HardReset})
		}
	} else {
		// Clone directly from the PR head branch. This avoids the issues
		// with fetching hidden refs (refs/pull/*) and plain init.
		r, err = git.PlainClone(targetPath, false, &git.CloneOptions{
			URL:           cloneURL,
			ReferenceName: headBranch,
			SingleBranch:  true,
			Auth:          auth,
		})
		if err != nil {
			return "", fmt.Errorf("clone %s: %w", headRef, err)
		}
	}

	// Fetch any new commits on the PR branch (for existing repos).
	remote, err := r.Remote("origin")
	if err == nil {
		_ = remote.Fetch(&git.FetchOptions{
			RefSpecs: []config.RefSpec{headRefSpec},
			Auth:     auth,
		})
	}

	// For existing repos, checkout the updated branch.
	if err := checkoutBranch(r, headBranch); err != nil {
		return "", fmt.Errorf("checkout %s: %w", headRef, err)
	}

	return targetPath, nil
}

// checkoutBranch checks out the given branch, creating it if it doesn't exist locally.
func checkoutBranch(r *git.Repository, branch plumbing.ReferenceName) error {
	wt, err := r.Worktree()
	if err != nil {
		return fmt.Errorf("get worktree: %w", err)
	}
	if err := wt.Checkout(&git.CheckoutOptions{
		Branch: branch,
		Force:  true,
	}); err != nil {
		return fmt.Errorf("checkout: %w", err)
	}
	return nil
}

// Cleanup removes the cloned repository directory for a specific PR.
func (m *CloneManager) Cleanup(owner, repo string, pullNumber int) error {
	return os.RemoveAll(m.RepoDir(owner, repo, pullNumber))
}

// makeAuth returns BasicAuth for private repos. Uses x-access-token as the
// username with the token as the password (GitHub's convention).
func makeAuth(token string) *http.BasicAuth {
	if token == "" {
		return nil
	}
	return &http.BasicAuth{
		Username: "x-access-token",
		Password: token,
	}
}

// isGitRepo returns true if the directory contains a .git subdirectory.
func isGitRepo(dir string) bool {
	info, err := os.Stat(filepath.Join(dir, ".git"))
	if err != nil {
		return false
	}
	return info.IsDir()
}
