package sync

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/config"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/object"
	gittransport "github.com/go-git/go-git/v5/plumbing/transport"
	githttp "github.com/go-git/go-git/v5/plumbing/transport/http"
)

type GitRepo struct {
	repo     *git.Repository
	repoPath string
}

type SyncDirection int

const (
	SyncNone SyncDirection = iota
	SyncPush
	SyncPull
	SyncConflict
	SyncSkipped // returned when a sync is already in flight
)

// CloneOrOpen opens the repo at repoPath, or clones it from the given URL.
func CloneOrOpen(repoPath, repoURL, branch, username, token string) (*GitRepo, error) {
	repo, err := git.PlainOpen(repoPath)
	if err == nil {
		return &GitRepo{repo: repo, repoPath: repoPath}, nil
	}

	if !errors.Is(err, git.ErrRepositoryNotExists) && !os.IsNotExist(err) {
		return nil, fmt.Errorf("open repo: %w", err)
	}

	if err := os.MkdirAll(filepath.Dir(repoPath), 0755); err != nil {
		return nil, fmt.Errorf("create parent dir: %w", err)
	}

	refName := plumbing.NewBranchReferenceName(branch)
	am := buildAuth(username, token)
	repo, err = git.PlainClone(repoPath, false, &git.CloneOptions{
		URL:           repoURL,
		Auth:          am,
		ReferenceName: refName,
		SingleBranch:  true,
	})
	if err != nil {
		if errors.Is(err, gittransport.ErrEmptyRemoteRepository) {
			return initEmpty(repoPath, repoURL)
		}
		return nil, fmt.Errorf("clone: %w", err)
	}

	return &GitRepo{repo: repo, repoPath: repoPath}, nil
}

func initEmpty(repoPath, repoURL string) (*GitRepo, error) {
	repo, err := git.PlainInit(repoPath, false)
	if err != nil {
		return nil, fmt.Errorf("init: %w", err)
	}
	mainRef := plumbing.NewBranchReferenceName("main")
	if err := repo.Storer.SetReference(plumbing.NewSymbolicReference(plumbing.HEAD, mainRef)); err != nil {
		return nil, fmt.Errorf("set HEAD to main: %w", err)
	}
	if _, err := repo.CreateRemote(&config.RemoteConfig{
		Name: "origin",
		URLs: []string{repoURL},
		Fetch: []config.RefSpec{
			config.RefSpec("+refs/heads/*:refs/remotes/origin/*"),
		},
	}); err != nil {
		return nil, fmt.Errorf("create remote: %w", err)
	}
	return &GitRepo{repo: repo, repoPath: repoPath}, nil
}

// OpenOrInitLocal opens a local-only sync directory: returns the existing
// repo if any, otherwise runs git init. No remote is configured.
func OpenOrInitLocal(repoPath string) (*GitRepo, error) {
	if err := os.MkdirAll(repoPath, 0755); err != nil {
		return nil, fmt.Errorf("create repo dir: %w", err)
	}
	if repo, err := git.PlainOpen(repoPath); err == nil {
		return &GitRepo{repo: repo, repoPath: repoPath}, nil
	} else if !errors.Is(err, git.ErrRepositoryNotExists) && !os.IsNotExist(err) {
		return nil, fmt.Errorf("open repo: %w", err)
	}
	repo, err := git.PlainInit(repoPath, false)
	if err != nil {
		return nil, fmt.Errorf("init: %w", err)
	}
	mainRef := plumbing.NewBranchReferenceName("main")
	if err := repo.Storer.SetReference(plumbing.NewSymbolicReference(plumbing.HEAD, mainRef)); err != nil {
		return nil, fmt.Errorf("set HEAD to main: %w", err)
	}
	return &GitRepo{repo: repo, repoPath: repoPath}, nil
}

// StageAndCommit stages all files and creates a commit. Returns true if committed.
func (g *GitRepo) StageAndCommit(msg string) (bool, error) {
	wt, err := g.repo.Worktree()
	if err != nil {
		return false, fmt.Errorf("worktree: %w", err)
	}

	status, err := wt.Status()
	if err != nil {
		return false, fmt.Errorf("status: %w", err)
	}
	if status.IsClean() {
		return false, nil
	}

	// Whitelist only known config filenames so stray files dropped in
	// the sync repo (e.g. an SSH key) are NOT committed plaintext
	// (SYNC-P1-9).
	for _, name := range []string{
		"connections.json", "settings.json",
		"ai-sessions.json", "skills.json",
		"quickCommands.json", ".sync-salt", "README.md",
	} {
		if _, err := os.Stat(filepath.Join(g.repoPath, name)); err != nil {
			continue
		}
		if _, err := wt.Add(name); err != nil {
			return false, fmt.Errorf("add %s: %w", name, err)
		}
	}

	_, err = wt.Commit(msg, &git.CommitOptions{
		Author: &object.Signature{
			Name:  "uniTerm",
			Email: "uniterm@local",
			When:  time.Now(),
		},
	})
	if err != nil {
		return false, fmt.Errorf("commit: %w", err)
	}
	return true, nil
}

func (g *GitRepo) Push(username, token string) error {
	err := g.repo.Push(&git.PushOptions{Auth: buildAuth(username, token)})
	if errors.Is(err, git.NoErrAlreadyUpToDate) {
		return nil
	}
	return err
}

func (g *GitRepo) Pull(username, token string) error {
	wt, err := g.repo.Worktree()
	if err != nil {
		return fmt.Errorf("worktree: %w", err)
	}
	return wt.Pull(&git.PullOptions{Auth: buildAuth(username, token), SingleBranch: true})
}

func (g *GitRepo) Fetch(username, token string) error {
	err := g.repo.Fetch(&git.FetchOptions{Auth: buildAuth(username, token), Force: true})
	if errors.Is(err, git.NoErrAlreadyUpToDate) {
		return nil
	}
	return err
}

// ReadRemoteFile reads a file from the remote tracking branch without touching the worktree.
func (g *GitRepo) ReadRemoteFile(branch, filePath string) ([]byte, error) {
	remoteRef, err := g.repo.Reference(
		plumbing.NewRemoteReferenceName("origin", branch), true,
	)
	if err != nil {
		return nil, err
	}
	commit, err := g.repo.CommitObject(remoteRef.Hash())
	if err != nil {
		return nil, err
	}
	tree, err := commit.Tree()
	if err != nil {
		return nil, err
	}
	file, err := tree.File(filePath)
	if err != nil {
		return nil, err
	}
	content, err := file.Contents()
	if err != nil {
		return nil, err
	}
	return []byte(content), nil
}

// CompareHeads returns sync direction after fetching.
func (g *GitRepo) CompareHeads(branch string) (SyncDirection, *time.Time, *time.Time, error) {
	localRef, err := g.repo.Head()
	if err != nil {
		return SyncNone, nil, nil, fmt.Errorf("local head: %w", err)
	}
	localHash := localRef.Hash()

	remoteRef, err := g.repo.Reference(
		plumbing.NewRemoteReferenceName("origin", branch), true,
	)
	if err != nil {
		if err == plumbing.ErrReferenceNotFound {
			return SyncPush, nil, nil, nil
		}
		return SyncNone, nil, nil, fmt.Errorf("remote ref: %w", err)
	}
	remoteHash := remoteRef.Hash()

	if localHash == remoteHash {
		return SyncNone, nil, nil, nil
	}

	localCommit, err := g.repo.CommitObject(localHash)
	if err != nil {
		return SyncNone, nil, nil, fmt.Errorf("local commit: %w", err)
	}
	remoteCommit, err := g.repo.CommitObject(remoteHash)
	if err != nil {
		return SyncNone, nil, nil, fmt.Errorf("remote commit: %w", err)
	}

	localTime := localCommit.Committer.When
	remoteTime := remoteCommit.Committer.When

	localAncestor, _ := localCommit.IsAncestor(remoteCommit)
	remoteAncestor, _ := remoteCommit.IsAncestor(localCommit)

	if remoteAncestor {
		return SyncPush, &localTime, &remoteTime, nil
	}
	if localAncestor {
		return SyncPull, &localTime, &remoteTime, nil
	}
	return SyncConflict, &localTime, &remoteTime, nil
}

// PushToBranch pushes the current HEAD to the specified remote branch.
func (g *GitRepo) PushToBranch(branch, username, token string) error {
	srcRef := plumbing.NewBranchReferenceName(branch)
	err := g.repo.Push(&git.PushOptions{
		Auth: buildAuth(username, token),
		RefSpecs: []config.RefSpec{
			config.RefSpec(fmt.Sprintf("%s:refs/heads/%s", srcRef, branch)),
		},
	})
	if errors.Is(err, git.NoErrAlreadyUpToDate) {
		return nil
	}
	return err
}

// ForcePush pushes with force, overwriting remote.
func (g *GitRepo) ForcePush(username, token string) error {
	return g.repo.Push(&git.PushOptions{Auth: buildAuth(username, token), Force: true})
}

// ResetToRemote resets local HEAD to match remote branch.
func (g *GitRepo) ResetToRemote(branch string) error {
	wt, err := g.repo.Worktree()
	if err != nil {
		return fmt.Errorf("worktree: %w", err)
	}
	remoteRef, err := g.repo.Reference(
		plumbing.NewRemoteReferenceName("origin", branch), true,
	)
	if err != nil {
		return fmt.Errorf("remote ref: %w", err)
	}
	return wt.Reset(&git.ResetOptions{
		Commit: remoteRef.Hash(),
		Mode:   git.HardReset,
	})
}

// TestConnection verifies the repo URL is reachable.
func TestConnection(repoURL, username, token string) error {
	remote := git.NewRemote(nil, &config.RemoteConfig{
		Name: "origin",
		URLs: []string{repoURL},
	})
	_, err := remote.List(&git.ListOptions{Auth: buildAuth(username, token)})
	if err != nil {
		return fmt.Errorf("remote unreachable: %w", err)
	}
	return nil
}

func buildAuth(username, token string) gittransport.AuthMethod {
	return &githttp.BasicAuth{
		Username: username,
		Password: token,
	}
}
