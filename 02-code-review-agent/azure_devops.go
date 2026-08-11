package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
)

// TestAzureConnection attempts to fetch basic PR details from Azure DevOps to verify the PAT works.
func TestAzureConnection(org, project, repo string, prID int, pat string) (string, error) {
	// Construct the Azure DevOps REST API URL for getting a Pull Request
	url := fmt.Sprintf("https://dev.azure.com/%s/%s/_apis/git/repositories/%s/pullrequests/%d?api-version=7.1", org, project, repo, prID)

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return "", fmt.Errorf("failed to create request: %w", err)
	}

	// Add Basic Authentication using the PAT
	// In Azure DevOps, the username can be anything (or empty), and the password is the PAT
	req.SetBasicAuth("", pat)
	req.Header.Add("Accept", "application/json")

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("failed to execute request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusUnauthorized {
		return "", fmt.Errorf("authentication failed (401). Please check if your PAT is valid and has 'Code (Read)' scope")
	}

	if resp.StatusCode == http.StatusNotFound {
		return "", fmt.Errorf("not found (404). Check if the Org, Project, Repo name, and PR ID are correct")
	}

	if resp.StatusCode != http.StatusOK {
		bodyBytes, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("API returned status %d: %s", resp.StatusCode, string(bodyBytes))
	}

	// Parse a small part of the response just to prove it worked
	var result struct {
		Title         string `json:"title"`
		State         string `json:"status"`
		SourceRefName string `json:"sourceRefName"`
	}
	
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", fmt.Errorf("failed to parse response JSON: %w", err)
	}

	fmt.Printf("✅ Successfully found PR #%d: \"%s\" (Status: %s, Source: %s)\n", prID, result.Title, result.State, result.SourceRefName)
	return result.SourceRefName, nil
}
