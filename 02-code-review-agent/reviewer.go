package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os/exec"
)

// ReviewComment represents a single actionable code review feedback from the AI
type ReviewComment struct {
	File    string `json:"file"`
	Line    int    `json:"line"`
	Comment string `json:"comment"`
}

// ReviewCode orchestrates the RAG pipeline by generating the PR diff, retrieving context
// from the Vector Database, and passing it to DeepSeek Coder for analysis.
func ReviewCode(tempDir, org, project, repoName, pat, sourceBranch, storyContext string, prID int) ([]ReviewComment, error) {
	fmt.Printf("\n🤖 Starting AI Code Review process for PR #%d...\n", prID)

	// 1. Generate the Diff using native Git
	diff, err := generateDiff(tempDir, org, project, repoName, pat, sourceBranch, prID)
	if err != nil {
		return nil, err
	}
	
	if len(diff) == 0 {
		fmt.Println("⚠️ The pull request diff is empty. Nothing to review!")
		return nil, nil
	}

	// 2. Fetch context from our ChromaDB Vector Database
	contextData, err := fetchContextFromDB(repoName, diff)
	if err != nil {
		fmt.Printf("⚠️ Warning: Could not retrieve context from Vector Database: %v\n", err)
		// We can still proceed without context if the DB fails
	}

	// 3. Send to AI via Ollama
	fmt.Println("🧠 Sending Diff and RAG Context to qwen2.5-coder:14b (This may take a minute)...")
	reviewComments, err := generateAIReview(diff, contextData, storyContext)
	if err != nil {
		return nil, err
	}

	fmt.Println("\n================= AI CODE REVIEW =================")
	for i, c := range reviewComments {
		fmt.Printf("Comment %d | File: %s | Line: %d\n%s\n\n", i+1, c.File, c.Line, c.Comment)
	}
	if len(reviewComments) == 0 {
		fmt.Println("AI found no issues! Code looks great.")
	}
	fmt.Println("==================================================")
	
	return reviewComments, nil
}

// generateDiff fetches the Pull Request branch from Azure DevOps and compares it against the base branch
func generateDiff(tempDir, org, project, repoName, pat, sourceBranch string, prID int) (string, error) {
	fmt.Println("📊 Fetching Pull Request changes from Azure DevOps...")

	gitURL := fmt.Sprintf("https://%s@dev.azure.com/%s/%s/_git/%s", pat, org, project, repoName)

	// Fetch the actual source branch of the PR
	fetchCmd := exec.Command("git", "fetch", gitURL, sourceBranch)
	fetchCmd.Dir = tempDir
	
	output, err := fetchCmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("failed to fetch PR source branch from ADO: %w\nOutput: %s", err, string(output))
	}

	// Generate the diff between the base branch (HEAD) and the fetched PR branch (FETCH_HEAD)
	diffCmd := exec.Command("git", "diff", "HEAD", "FETCH_HEAD")
	diffCmd.Dir = tempDir
	
	diffOutput, diffErr := diffCmd.CombinedOutput()
	if diffErr != nil {
		// git diff returns exit code 1 if there are differences, so we only error if it's not a normal diff output
		if len(diffOutput) == 0 {
			return "", fmt.Errorf("failed to generate git diff: %w", diffErr)
		}
	}

	return string(diffOutput), nil
}

// fetchContextFromDB queries ChromaDB for related code files to help the AI understand the changes
func fetchContextFromDB(repoName string, diff string) (string, error) {
	fmt.Println("🔎 Querying Vector Database for related codebase context...")

	// 1. Embed the diff to use as our search query
	// We truncate the diff to the first 2000 chars so we don't overload the embedding model
	queryText := diff
	if len(queryText) > 2000 {
		queryText = queryText[:2000]
	}

	reqBody, _ := json.Marshal(map[string]interface{}{
		"model":  "nomic-embed-text",
		"prompt": queryText,
	})

	resp, err := http.Post("http://localhost:11434/api/embeddings", "application/json", bytes.NewBuffer(reqBody))
	if err != nil {
		return "", err
	}
	
	var embResult struct {
		Embedding []float64 `json:"embedding"`
	}
	bodyBytes, _ := io.ReadAll(resp.Body)
	json.Unmarshal(bodyBytes, &embResult)
	resp.Body.Close()

	if len(embResult.Embedding) == 0 {
		return "", fmt.Errorf("embedding was empty")
	}

	// 2. Query ChromaDB using the new v2 API
	collectionName := fmt.Sprintf("code-review-%s", repoName)
	baseChromaURL := "http://localhost:8000/api/v2/tenants/default_tenant/databases/default_database/collections"

	// We need the Collection ID first
	respCol, err := http.Get(baseChromaURL + "/" + collectionName)
	if err != nil {
		return "", err
	}
	var colResult struct {
		ID string `json:"id"`
	}
	colBytes, _ := io.ReadAll(respCol.Body)
	json.Unmarshal(colBytes, &colResult)
	respCol.Body.Close()

	if colResult.ID == "" {
		return "", fmt.Errorf("collection ID not found")
	}

	// Query the collection
	queryURL := fmt.Sprintf("%s/%s/query", baseChromaURL, colResult.ID)
	queryBody, _ := json.Marshal(map[string]interface{}{
		"query_embeddings": [][]float64{embResult.Embedding},
		"n_results":        3, // Get the top 3 most relevant code chunks
	})

	respQuery, err := http.Post(queryURL, "application/json", bytes.NewBuffer(queryBody))
	if err != nil {
		return "", err
	}
	defer respQuery.Body.Close()
	
	var queryResult struct {
		Documents [][]string `json:"documents"`
	}
	qBytes, _ := io.ReadAll(respQuery.Body)
	json.Unmarshal(qBytes, &queryResult)

	// Combine the retrieved context chunks into a single string
	var contextString string
	if len(queryResult.Documents) > 0 && len(queryResult.Documents[0]) > 0 {
		for i, doc := range queryResult.Documents[0] {
			contextString += fmt.Sprintf("\n--- Context Chunk %d ---\n%s\n", i+1, doc)
		}
	}

	return contextString, nil
}

// generateAIReview calls DeepSeek Coder via Ollama's chat API
func generateAIReview(diff string, codeContext string, storyContext string) ([]ReviewComment, error) {
	storyText := ""
	if storyContext != "" {
		storyText = fmt.Sprintf("\n\nHere is the Business Requirement / Story / Bug this PR is supposed to fix:\n%s\nPlease heavily scrutinize if the code changes accurately and securely fulfill this requirement.", storyContext)
	}

	prompt := fmt.Sprintf(`You are an expert Senior Software Engineer performing a strict Code Review.

Here is the Git Diff of the changes made by the developer:
%s

Here is some additional surrounding codebase context (retrieved via Vector Database) to help you understand the project architecture:
%s%s

INSTRUCTIONS:
Provide a highly specific, professional code review in JSON format.
DO NOT write a generic summary of the changes. You MUST map your feedback directly to the actual files and line numbers in the diff.
CRITICAL RULE: You must ONLY review files and lines that are present in the Git Diff. DO NOT write review comments for files or code that are only shown in the "Surrounding Codebase Context". The context is ONLY for your reference to understand how the diff fits into the larger architecture.
CRITICAL RULE 2: Act like a human Senior Developer. DO NOT mention "the provided context" in your comments. If something is missing or incorrect in the diff, directly tell the developer what needs to be fixed.

FORMATTING REQUIREMENTS:
1. You MUST return ONLY a valid JSON object containing a "reviews" key.
2. The "reviews" key MUST be an array of objects, where each object has:
   - "file": The EXACT file path as shown in the git diff (e.g. if the diff says "diff --git a/src/app.tsx b/src/app.tsx", you MUST output "/src/app.tsx"). DO NOT invent file names.
   - "line": The exact line number in the diff where the issue exists (e.g. 42). If the issue is general to the file, use line 1.
   - "comment": Your actionable feedback, including code snippets showing how to fix it.
3. Check for logic errors, unused imports, edge cases, and performance problems.
4. If a Story/Bug was provided, explicitly state if the code successfully resolves the ticket in one of the comments.
5. If the code is completely flawless, return: {"reviews": []}

EXAMPLE OUTPUT:
{
  "reviews": [
    {
      "file": "src/app/components/TimesheetStatus.tsx",
      "line": 15,
      "comment": "This component is unused and should be removed according to the refactoring story."
    },
    {
      "file": "src/utils/helpers.ts",
      "line": 42,
      "comment": "You can optimize this loop:\n\n    // optimized code\n"
    }
  ]
}`, diff, codeContext, storyText)

	reqBody, _ := json.Marshal(map[string]interface{}{
		"model":  "qwen2.5-coder:14b",
		"messages": []map[string]string{
			{
				"role":    "user",
				"content": prompt,
			},
		},
		"format": "json", // Force Ollama to return JSON
		"stream": false,
	})

	resp, err := http.Post("http://localhost:11434/api/chat", "application/json", bytes.NewBuffer(reqBody))
	if err != nil {
		return nil, fmt.Errorf("failed to contact Ollama for AI review: %w", err)
	}
	defer resp.Body.Close()

	bodyBytes, _ := io.ReadAll(resp.Body)

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("Ollama API returned an error (status %d): %s", resp.StatusCode, string(bodyBytes))
	}

	var chatResult struct {
		Message struct {
			Content string `json:"content"`
		} `json:"message"`
		Error string `json:"error"`
	}
	
	if err := json.Unmarshal(bodyBytes, &chatResult); err != nil {
		return nil, fmt.Errorf("failed to parse AI response: %w", err)
	}

	if chatResult.Error != "" {
		return nil, fmt.Errorf("Ollama returned an error: %s", chatResult.Error)
	}

	var aiResponse struct {
		Reviews []ReviewComment `json:"reviews"`
		Results []ReviewComment `json:"results"` // fallback in case the AI uses "results" instead of "reviews"
	}

	if err := json.Unmarshal([]byte(chatResult.Message.Content), &aiResponse); err != nil {
		fmt.Printf("\n⚠️ Warning: The local AI model returned an invalid JSON schema instead of an array. Skipping inline comments.\nModel Output: %s\n", chatResult.Message.Content)
		return nil, nil // Gracefully return an empty array instead of crashing
	}

	comments := aiResponse.Reviews
	if len(aiResponse.Results) > 0 {
		comments = append(comments, aiResponse.Results...)
	}

	return comments, nil
}
