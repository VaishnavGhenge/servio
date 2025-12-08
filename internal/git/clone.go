package git

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// CloneRepository clones a git repository to the specified directory
// If repoURL is empty, this function does nothing
func CloneRepository(repoURL, targetDir string) error {
	if repoURL == "" {
		return nil
	}

	// Validate URL format
	if !isValidGitURL(repoURL) {
		return fmt.Errorf("invalid git repository URL: %s", repoURL)
	}

	// Check if directory exists
	if _, err := os.Stat(targetDir); err == nil {
		// Directory exists - check if it's a git repo
		gitDir := filepath.Join(targetDir, ".git")
		if _, err := os.Stat(gitDir); err == nil {
			// It's already a git repo, try to pull latest
			return pullRepository(targetDir)
		}
		// Directory exists but not a git repo
		return fmt.Errorf("directory %s already exists and is not a git repository", targetDir)
	}

	// Create parent directory if needed
	parentDir := filepath.Dir(targetDir)
	if err := os.MkdirAll(parentDir, 0755); err != nil {
		return fmt.Errorf("failed to create parent directory: %w", err)
	}

	// Clone the repository
	cmd := exec.Command("git", "clone", repoURL, targetDir)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("git clone failed: %w\nOutput: %s", err, string(output))
	}

	return nil
}

// pullRepository pulls the latest changes from the remote repository
func pullRepository(repoDir string) error {
	cmd := exec.Command("git", "-C", repoDir, "pull", "--ff-only")
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("git pull failed: %w\nOutput: %s", err, string(output))
	}
	return nil
}

// isValidGitURL checks if the URL is a valid git repository URL
func isValidGitURL(url string) bool {
	if url == "" {
		return false
	}

	// Check for common git URL patterns
	patterns := []string{
		"git@",           // SSH: git@github.com:user/repo.git
		"https://",       // HTTPS: https://github.com/user/repo.git
		"http://",        // HTTP: http://github.com/user/repo.git
		"ssh://",         // SSH: ssh://git@github.com/user/repo.git
		"git://",         // Git protocol: git://github.com/user/repo.git
	}

	for _, pattern := range patterns {
		if strings.HasPrefix(url, pattern) {
			return true
		}
	}

	return false
}

// UpdateRepository performs a git pull in the specified directory
func UpdateRepository(repoDir string) error {
	// Check if directory exists and is a git repo
	gitDir := filepath.Join(repoDir, ".git")
	if _, err := os.Stat(gitDir); os.IsNotExist(err) {
		return fmt.Errorf("directory %s is not a git repository", repoDir)
	}

	return pullRepository(repoDir)
}
