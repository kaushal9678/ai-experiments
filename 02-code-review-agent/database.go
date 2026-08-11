package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/tmc/langchaingo/textsplitter"
)

// IndexRepository walks through the cloned repository, chunks the files,
// generates embeddings via Ollama, and stores them in ChromaDB.
func IndexRepository(repoPath, repoName string) error {
	fmt.Println("🔍 Scanning repository files for indexing...")

	splitter := textsplitter.NewRecursiveCharacter()
	splitter.ChunkSize = 1000
	splitter.ChunkOverlap = 200

	var documents []string
	var ids []string
	var metadatas []map[string]interface{}
	chunkCounter := 1

	err := filepath.Walk(repoPath, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		if info.IsDir() && info.Name() == ".git" {
			return filepath.SkipDir
		}

		if info.IsDir() {
			return nil
		}

		if strings.HasSuffix(info.Name(), ".png") || strings.HasSuffix(info.Name(), ".exe") {
			return nil
		}

		content, err := os.ReadFile(path)
		if err != nil {
			return nil // Skip file on error
		}

		fileText := string(content)
		if len(strings.TrimSpace(fileText)) == 0 {
			return nil
		}

		chunks, err := splitter.SplitText(fileText)
		if err != nil {
			return nil
		}

		for i, chunk := range chunks {
			documents = append(documents, chunk)
			
			relativeFilePath := strings.TrimPrefix(path, repoPath)
			metadatas = append(metadatas, map[string]interface{}{
				"source": relativeFilePath,
				"chunk":  i,
			})
			
			ids = append(ids, fmt.Sprintf("%s-chunk-%d", relativeFilePath, chunkCounter))
			chunkCounter++
		}

		return nil
	})

	if err != nil {
		return fmt.Errorf("failed to walk repository: %w", err)
	}

	fmt.Printf("✅ Parsed repository into %d chunks. Now generating Embeddings using Ollama...\n", len(documents))

	if len(documents) == 0 {
		return fmt.Errorf("no valid code files found to index")
	}

	// 1. Generate Embeddings using Ollama REST API directly
	var embeddings [][]float64
	for i, doc := range documents {
		if i%10 == 0 {
			fmt.Printf("  -> Embedding chunk %d of %d...\n", i+1, len(documents))
		}
		
		reqBody, _ := json.Marshal(map[string]interface{}{
			"model":  "mxbai-embed-large",
			"prompt": doc,
		})

		resp, err := http.Post("http://localhost:11434/api/embeddings", "application/json", bytes.NewBuffer(reqBody))
		if err != nil {
			return fmt.Errorf("failed to reach Ollama: %w", err)
		}
		
		var result struct {
			Embedding []float64 `json:"embedding"`
		}
		bodyBytes, _ := io.ReadAll(resp.Body)
		json.Unmarshal(bodyBytes, &result)
		resp.Body.Close()
		
		embeddings = append(embeddings, result.Embedding)
	}

	// 2. Connect to ChromaDB REST API directly
	collectionName := fmt.Sprintf("code-review-%s", repoName)
	fmt.Printf("🚀 Connecting to ChromaDB to create collection '%s'...\n", collectionName)
	
	// The new ChromaDB API requires tenant and database paths
	baseChromaURL := "http://localhost:8000/api/v2/tenants/default_tenant/databases/default_database/collections"

	// Delete collection if it exists
	req, _ := http.NewRequest(http.MethodDelete, baseChromaURL+"/"+collectionName, nil)
	client := &http.Client{}
	client.Do(req) 

	// Create new collection
	createColBody, _ := json.Marshal(map[string]interface{}{
		"name": collectionName,
	})
	resp, err := http.Post(baseChromaURL, "application/json", bytes.NewBuffer(createColBody))
	if err != nil {
		return fmt.Errorf("failed to reach ChromaDB: %w\n(Did you start the Chroma server?)", err)
	}
	
	var colResult struct {
		ID string `json:"id"`
	}
	bodyBytes, _ := io.ReadAll(resp.Body)
	json.Unmarshal(bodyBytes, &colResult)
	resp.Body.Close()

	if colResult.ID == "" {
		return fmt.Errorf("failed to create ChromaDB collection, response: %s", string(bodyBytes))
	}

	// Add documents to collection in batches of 100 to prevent "broken pipe" payload limits
	batchSize := 100
	addURL := fmt.Sprintf("%s/%s/add", baseChromaURL, colResult.ID)

	fmt.Printf("🚀 Saving %d chunks to ChromaDB in batches of %d...\n", len(documents), batchSize)

	for i := 0; i < len(documents); i += batchSize {
		end := i + batchSize
		if end > len(documents) {
			end = len(documents)
		}

		addDocsBody, _ := json.Marshal(map[string]interface{}{
			"ids":        ids[i:end],
			"documents":  documents[i:end],
			"embeddings": embeddings[i:end],
			"metadatas":  metadatas[i:end],
		})
		
		respAdd, err := http.Post(addURL, "application/json", bytes.NewBuffer(addDocsBody))
		if err != nil {
			return fmt.Errorf("failed to add documents to ChromaDB at batch %d: %w", i, err)
		}
		
		// If it's not 200/201, we should probably read the error but let's just close body for now
		if respAdd.StatusCode >= 400 {
			bodyBytes, _ := io.ReadAll(respAdd.Body)
			respAdd.Body.Close()
			return fmt.Errorf("ChromaDB rejected batch %d: %s", i, string(bodyBytes))
		}
		respAdd.Body.Close()
	}

	fmt.Printf("🎯 Successfully indexed all code into Vector Database collection: '%s'!\n", collectionName)
	return nil
}
