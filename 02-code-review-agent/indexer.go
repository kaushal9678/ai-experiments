package main

import (
	"fmt"
	"os"
	"os/exec"
)

// CloneRepository securely clones the repository into a temporary directory using the system's Git CLI.
func CloneRepository(org, project, repo, branch, pat string) (string, error) {
	// 1. Create a secure temporary directory
	tempDir, err := os.MkdirTemp("", "code-review-*")
	if err != nil {
		return "", fmt.Errorf("failed to create temp directory: %w", err)
	}

	fmt.Printf("📦 Cloning branch '%s' from repository '%s' into temporary folder...\n", branch, repo)

	// 2. Construct the Git HTTPS URL with the PAT embedded securely
	// Format: https://{pat}@dev.azure.com/{org}/{project}/_git/{repo}
	gitURL := fmt.Sprintf("https://%s@dev.azure.com/%s/%s/_git/%s", pat, org, project, repo)

	// 3. Use the system's native git CLI to clone the repository
	// This avoids "400 Bad Request" git-upload-pack issues common with third-party Git libraries
	cmd := exec.Command("git", "clone", "--depth=1", "--branch", branch, gitURL, tempDir)
	
	// Capture output in case of error, but hide the PAT from logs
	output, err := cmd.CombinedOutput()
	if err != nil {
		os.RemoveAll(tempDir)
		return "", fmt.Errorf("native git clone failed: %w (output: %s)", err, string(output))
	}

	fmt.Println("✅ Repository successfully cloned!")
	return tempDir, nil
}

// Cleanup removes the temporary directory after the review is complete.
func Cleanup(tempDir string) {
	if tempDir != "" {
		fmt.Printf("🧹 Cleaning up temporary files in %s...\n", tempDir)
		err := os.RemoveAll(tempDir)
		if err != nil {
			fmt.Printf("Warning: Failed to clean up temp directory: %v\n", err)
		}
	}
}
