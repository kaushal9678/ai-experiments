package main

import (
	"flag"
	"fmt"
	"log"
	"os"

	"github.com/joho/godotenv"
)

func main() {
	// Load environment variables from .env
	err := godotenv.Load()
	if err != nil {
		log.Println("Warning: Error loading .env file (maybe it doesn't exist yet?)")
	}

	// Parse command line arguments
	repoFlag := flag.String("repo", "", "The target Azure DevOps repository name")
	prFlag := flag.Int("pr", 0, "The Pull Request ID to review")
	branchFlag := flag.String("branch", "develop", "The target branch for context (default: develop)")
	storyFlag := flag.String("story", "", "Optional: The User Story description that this PR is supposed to solve")
	taskFlag := flag.String("task", "", "Optional: The Task description that this PR is supposed to solve")
	bugFlag := flag.String("bug", "", "Optional: The Bug description that this PR is supposed to solve")
	flag.Parse()

	if *repoFlag == "" || *prFlag == 0 {
		fmt.Println("Usage: go run main.go --repo=\"Repo-A\" --pr=102")
		os.Exit(1)
	}

	// Load config from environment
	org := os.Getenv("AZURE_DEVOPS_ORG")
	project := os.Getenv("AZURE_DEVOPS_PROJECT")
	pat := os.Getenv("AZURE_DEVOPS_PAT")

	if org == "" || project == "" || pat == "" {
		log.Fatal("Error: Please make sure AZURE_DEVOPS_ORG, AZURE_DEVOPS_PROJECT, and AZURE_DEVOPS_PAT are set in your .env file")
	}

	fmt.Printf("Starting Code Review Agent for PR #%d in repository '%s'...\n", *prFlag, *repoFlag)
	fmt.Println("--------------------------------------------------")

	// 1. Test the connection to Azure DevOps
	fmt.Println("Testing connection to Azure DevOps API...")
	sourceBranch, err := TestAzureConnection(org, project, *repoFlag, *prFlag, pat)
	if err != nil {
		log.Fatalf("Connection failed: %v\n", err)
	}
	fmt.Println("✅ Connection successful!")
	fmt.Println("--------------------------------------------------")

	// 2. Clone the Repository locally (using the target branch)
	tempDir, err := CloneRepository(org, project, *repoFlag, *branchFlag, pat)
	if err != nil {
		log.Fatalf("Failed to clone repository: %v\n", err)
	}
	// Ensure we clean up the temporary directory when the program exits
	defer Cleanup(tempDir)
	
	fmt.Printf("The code has been successfully cloned to: %s\n", tempDir)
	
	// 3. Index the Repository (Chunking and Vector DB)
	err = IndexRepository(tempDir, *repoFlag)
	if err != nil {
		log.Fatalf("Failed to index repository: %v\n", err)
	}

	// Combine requirement contexts
	reqContext := *storyFlag
	if *taskFlag != "" {
		reqContext += " " + *taskFlag
	}
	if *bugFlag != "" {
		reqContext += " " + *bugFlag
	}

	// 4. Run the AI Code Review!
	review, err := ReviewCode(tempDir, org, project, *repoFlag, pat, sourceBranch, reqContext, *prFlag)
	if err != nil {
		log.Fatalf("Failed to review code: %v\n", err)
	}

	// 5. Publish the Review to Azure DevOps
	if len(review) > 0 {
		err = PostReviewComment(org, project, *repoFlag, pat, *prFlag, review)
		if err != nil {
			log.Fatalf("Failed to post comment to PR: %v\n", err)
		}
	}
	
	fmt.Println("\n🎉 Code Review Agent has successfully finished its job!")
}
