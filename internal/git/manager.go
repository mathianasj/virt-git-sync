/*
Copyright 2026 Joshua Mathianas.

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

package git

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/config"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/object"
	"github.com/go-git/go-git/v5/plumbing/transport"
	"github.com/go-git/go-git/v5/plumbing/transport/http"
	"github.com/go-git/go-git/v5/plumbing/transport/ssh"
	gossh "golang.org/x/crypto/ssh"
	corev1 "k8s.io/api/core/v1"
)

// Manager handles git operations for VM YAML synchronization
type Manager struct {
	workDir string
	repoURL string
	branch  string
	auth    transport.AuthMethod
	repo    *git.Repository
}

// NewManager creates a new git manager
func NewManager(workDir, repoURL, branch string, secret *corev1.Secret) (*Manager, error) {
	if workDir == "" {
		return nil, fmt.Errorf("workDir is required")
	}
	if repoURL == "" {
		return nil, fmt.Errorf("repoURL is required")
	}
	if branch == "" {
		branch = "main"
	}

	mgr := &Manager{
		workDir: workDir,
		repoURL: repoURL,
		branch:  branch,
	}

	// Setup authentication if secret provided
	if secret != nil {
		if err := mgr.setupAuth(secret); err != nil {
			return nil, fmt.Errorf("failed to setup auth: %w", err)
		}
	}

	return mgr, nil
}

// setupAuth configures authentication from Kubernetes secret
func (m *Manager) setupAuth(secret *corev1.Secret) error {
	// Try SSH authentication first
	if sshKey, ok := secret.Data["ssh-private-key"]; ok {
		publicKeys, err := ssh.NewPublicKeys("git", sshKey, "")
		if err != nil {
			return fmt.Errorf("failed to parse SSH key: %w", err)
		}

		// Check for known_hosts (optional)
		if knownHosts, ok := secret.Data["known_hosts"]; ok {
			// Use provided known_hosts for strict validation
			publicKeys.HostKeyCallback, err = ssh.NewKnownHostsCallback(string(knownHosts))
			if err != nil {
				return fmt.Errorf("failed to parse known_hosts: %w", err)
			}
		} else {
			// Skip host key validation (insecure but common)
			// SECURITY: Using insecure host key validation - provide known_hosts in secret for strict validation
			fmt.Fprintf(os.Stderr, "WARNING: SSH host key validation disabled for git repository %s (no known_hosts provided)\n", m.repoURL)
			publicKeys.HostKeyCallback = gossh.InsecureIgnoreHostKey() //nolint:gosec // Audited: logged to stderr
		}

		m.auth = publicKeys
		return nil
	}

	// Try HTTP basic auth (username + password/token)
	if username, ok := secret.Data["username"]; ok {
		password := secret.Data["password"]
		m.auth = &http.BasicAuth{
			Username: string(username),
			Password: string(password),
		}
		return nil
	}

	return fmt.Errorf("secret must contain either 'ssh-private-key' or 'username'+'password'")
}

// Clone clones the repository to workDir
func (m *Manager) Clone(ctx context.Context) error {
	// Create parent directory if needed
	if err := os.MkdirAll(filepath.Dir(m.workDir), 0750); err != nil {
		return fmt.Errorf("failed to create parent directory: %w", err)
	}

	// Clone repository
	repo, err := git.PlainCloneContext(ctx, m.workDir, false, &git.CloneOptions{
		URL:           m.repoURL,
		Auth:          m.auth,
		ReferenceName: getBranchReference(m.branch),
		SingleBranch:  true,
		Depth:         1, // Shallow clone for performance
		Progress:      nil,
	})
	if err != nil {
		return fmt.Errorf("failed to clone repository: %w", err)
	}

	m.repo = repo
	return nil
}

// Open opens an existing repository at workDir
func (m *Manager) Open() error {
	repo, err := git.PlainOpen(m.workDir)
	if err != nil {
		return fmt.Errorf("failed to open repository: %w", err)
	}

	m.repo = repo
	return nil
}

// Pull fetches the latest changes from the remote
func (m *Manager) Pull(ctx context.Context) error {
	if m.repo == nil {
		return fmt.Errorf("repository not initialized, call Clone first")
	}

	// Fetch latest changes
	err := m.repo.FetchContext(ctx, &git.FetchOptions{
		Auth:       m.auth,
		RemoteName: "origin",
		RefSpecs: []config.RefSpec{
			config.RefSpec("+refs/heads/" + m.branch + ":refs/remotes/origin/" + m.branch),
		},
	})

	// Ignore already up-to-date
	if err != nil && err != git.NoErrAlreadyUpToDate {
		return fmt.Errorf("failed to fetch: %w", err)
	}

	// Get worktree
	worktree, err := m.repo.Worktree()
	if err != nil {
		return fmt.Errorf("failed to get worktree: %w", err)
	}

	// Reset to remote branch (hard reset to avoid conflicts)
	remoteBranch := plumbing.NewRemoteReferenceName("origin", m.branch)
	remoteRef, err := m.repo.Reference(remoteBranch, true)
	if err != nil {
		return fmt.Errorf("failed to get remote reference: %w", err)
	}

	err = worktree.Reset(&git.ResetOptions{
		Commit: remoteRef.Hash(),
		Mode:   git.HardReset,
	})
	if err != nil {
		return fmt.Errorf("failed to reset to remote: %w", err)
	}

	return nil
}

// WriteFile writes content to a file within the repository
func (m *Manager) WriteFile(path string, content []byte) error {
	if m.repo == nil {
		return fmt.Errorf("repository not initialized, call Clone first")
	}

	// Build full path
	fullPath := filepath.Join(m.workDir, path)

	// Create parent directories if needed
	dir := filepath.Dir(fullPath)
	if err := os.MkdirAll(dir, 0750); err != nil {
		return fmt.Errorf("failed to create directory %s: %w", dir, err)
	}

	// Write file
	if err := os.WriteFile(fullPath, content, 0600); err != nil {
		return fmt.Errorf("failed to write file %s: %w", path, err)
	}

	return nil
}

// ListFiles lists all files matching a pattern in the working directory
func (m *Manager) ListFiles(pattern string) ([]string, error) {
	if m.repo == nil {
		return nil, fmt.Errorf("repository not initialized, call Clone first")
	}

	// Build full pattern path
	fullPattern := filepath.Join(m.workDir, pattern)

	// Use glob to find matching files
	matches, err := filepath.Glob(fullPattern)
	if err != nil {
		return nil, fmt.Errorf("failed to glob files: %w", err)
	}

	// Convert absolute paths to relative paths
	relPaths := make([]string, 0, len(matches))
	for _, match := range matches {
		relPath, err := filepath.Rel(m.workDir, match)
		if err != nil {
			continue
		}
		relPaths = append(relPaths, relPath)
	}

	return relPaths, nil
}

// DeleteFile removes a file from the repository
func (m *Manager) DeleteFile(path string) error {
	if m.repo == nil {
		return fmt.Errorf("repository not initialized, call Clone first")
	}

	// Build full path
	fullPath := filepath.Join(m.workDir, path)

	// Remove file
	if err := os.Remove(fullPath); err != nil {
		if os.IsNotExist(err) {
			// File already deleted, not an error
			return nil
		}
		return fmt.Errorf("failed to delete file %s: %w", path, err)
	}

	// Also remove parent directory if empty
	dir := filepath.Dir(fullPath)
	entries, err := os.ReadDir(dir)
	if err == nil && len(entries) == 0 {
		_ = os.Remove(dir) // Ignore error, best effort
	}

	return nil
}

// CommitAndPush commits changes and pushes to remote
func (m *Manager) CommitAndPush(ctx context.Context, message string, files []string) error {
	if m.repo == nil {
		return fmt.Errorf("repository not initialized, call Clone first")
	}

	// Get worktree
	worktree, err := m.repo.Worktree()
	if err != nil {
		return fmt.Errorf("failed to get worktree: %w", err)
	}

	// Add files to staging
	for _, file := range files {
		_, err := worktree.Add(file)
		if err != nil {
			return fmt.Errorf("failed to add file %s: %w", file, err)
		}
	}

	// Check if there are changes to commit
	status, err := worktree.Status()
	if err != nil {
		return fmt.Errorf("failed to get status: %w", err)
	}

	if status.IsClean() {
		// No changes to commit
		return nil
	}

	// Commit changes
	_, err = worktree.Commit(message, &git.CommitOptions{
		Author: &object.Signature{
			Name:  "VirtGitSync Operator",
			Email: "virt-git-sync@kubernetes.local",
		},
	})
	if err != nil {
		return fmt.Errorf("failed to commit: %w", err)
	}

	// Push to remote
	err = m.repo.PushContext(ctx, &git.PushOptions{
		Auth:       m.auth,
		RemoteName: "origin",
		RefSpecs: []config.RefSpec{
			config.RefSpec("refs/heads/" + m.branch + ":refs/heads/" + m.branch),
		},
		Force: false,
	})

	if err != nil && err != git.NoErrAlreadyUpToDate {
		return fmt.Errorf("failed to push: %w", err)
	}

	return nil
}

// GetLastCommit returns the SHA of the last commit
func (m *Manager) GetLastCommit() (string, error) {
	if m.repo == nil {
		return "", fmt.Errorf("repository not initialized, call Clone first")
	}

	ref, err := m.repo.Head()
	if err != nil {
		return "", fmt.Errorf("failed to get HEAD: %w", err)
	}

	return ref.Hash().String(), nil
}

// HasUncommittedChanges checks if there are any uncommitted changes in the working tree
func (m *Manager) HasUncommittedChanges() (bool, error) {
	if m.repo == nil {
		return false, fmt.Errorf("repository not initialized, call Clone first")
	}

	worktree, err := m.repo.Worktree()
	if err != nil {
		return false, fmt.Errorf("failed to get worktree: %w", err)
	}

	status, err := worktree.Status()
	if err != nil {
		return false, fmt.Errorf("failed to get status: %w", err)
	}

	// If status is not clean, there are uncommitted changes
	return !status.IsClean(), nil
}

// getBranchReference converts branch name to plumbing reference
func getBranchReference(branch string) plumbing.ReferenceName {
	if strings.HasPrefix(branch, "refs/") {
		return plumbing.ReferenceName(branch)
	}
	return plumbing.NewBranchReferenceName(branch)
}
