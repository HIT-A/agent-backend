package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"time"
)

// Test RAG Pipeline with HIT-A/HITA_RagData

type SkillInvokeRequest struct {
	Input map[string]any `json:"input"`
}

type SkillInvokeResponse struct {
	Ok    bool           `json:"ok"`
	Data  map[string]any `json:"data,omitempty"`
	Error *struct {
		Code    string `json:"code"`
		Message string `json:"message"`
	} `json:"error,omitempty"`
}

func main() {
	apiURL := os.Getenv("API_URL")
	if apiURL == "" {
		apiURL = "http://localhost:8080"
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	fmt.Println("=== RAG Pipeline Test ===")
	fmt.Printf("API URL: %s\n\n", apiURL)

	// Step 1: Health check
	fmt.Println("Step 1: Health check...")
	resp, err := http.Get(apiURL + "/health")
	if err != nil {
		fmt.Printf("❌ Server not responding: %v\n", err)
		fmt.Println("\nTo start the server:")
		fmt.Println("  cd hoa-agent-backend && go run ./cmd/server")
		os.Exit(1)
	}
	resp.Body.Close()
	fmt.Println("✅ Server is healthy\n")

	// Step 2: List skills
	fmt.Println("Step 2: List available skills...")
	skillsResp, err := http.Get(apiURL + "/v1/skills")
	if err != nil {
		fmt.Printf("❌ Failed to list skills: %v\n", err)
		os.Exit(1)
	}

	var skillsData struct {
		Skills []struct {
			Name    string `json:"name"`
			IsAsync bool   `json:"is_async"`
		} `json:"skills"`
	}
	json.NewDecoder(skillsResp.Body).Decode(&skillsData)
	skillsResp.Body.Close()

	fmt.Printf("Found %d skills:\n", len(skillsData.Skills))
	for _, s := range skillsData.Skills {
		async := ""
		if s.IsAsync {
			async = " (async)"
		}
		fmt.Printf("  - %s%s\n", s.Name, async)
	}
	fmt.Println()

	// Step 3: Test RAG query before ingestion
	fmt.Println("Step 3: Test RAG query (before ingestion)...")
	queryResp := invokeSkill(ctx, apiURL, "rag.query", map[string]any{
		"query": "哈工大选课",
		"top_k": 3,
	})
	if queryResp != nil {
		fmt.Printf("Result: %+v\n\n", queryResp)
	}

	// Step 4: Ingest from GitHub
	fmt.Println("Step 4: Ingest HIT-A/HITA_RagData...")
	ingestResp := invokeSkill(ctx, apiURL, "rag.ingest", map[string]any{
		"repo":         "HIT-A/HITA_RagData",
		"ref":          "main",
		"path_prefix":  "新生手册",
		"source":       "hit-freshman-guide",
		"workers":      2,
		"store_in_cos": false,
		"max_files":    27,
		"max_chunks":   500,
	})

	if ingestResp != nil {
		if jobID, ok := ingestResp["job_id"].(string); ok {
			fmt.Printf("Job started: %s\n", jobID)
			fmt.Println("Waiting for job to complete...")

			// Poll for job status
			for i := 0; i < 30; i++ {
				time.Sleep(2 * time.Second)
				jobStatus := getJobStatus(ctx, apiURL, jobID)
				fmt.Printf("  Status: %s\n", jobStatus["status"])

				if jobStatus["status"] == "succeeded" {
					fmt.Printf("\n✅ Job completed!\n")
					if result, ok := jobStatus["result"].(map[string]any); ok {
						fmt.Printf("  Processed files: %v\n", result["processed_files"])
						fmt.Printf("  Processed chunks: %v\n", result["processed_chunks"])
						fmt.Printf("  Upserted points: %v\n", result["upserted_points"])
					}
					break
				} else if jobStatus["status"] == "failed" {
					fmt.Printf("❌ Job failed: %v\n", jobStatus["error"])
					break
				}
			}
		}
	}
	fmt.Println()

	// Step 5: Test RAG query after ingestion
	fmt.Println("Step 5: Test RAG query (after ingestion)...")
	queryResp2 := invokeSkill(ctx, apiURL, "rag.query", map[string]any{
		"query": "哈工大选课",
		"top_k": 5,
	})
	if queryResp2 != nil {
		if hits, ok := queryResp2["hits"].([]any); ok {
			fmt.Printf("Found %d hits:\n", len(hits))
			for i, h := range hits {
				if hit, ok := h.(map[string]any); ok {
					fmt.Printf("\n[%d] Score: %.3f\n", i+1, hit["score"].(float64))
					fmt.Printf("    Title: %s\n", hit["title"])
					fmt.Printf("    Source: %s\n", hit["source"])
					if snippet, ok := hit["snippet"].(string); ok {
						if len(snippet) > 100 {
							snippet = snippet[:100] + "..."
						}
						fmt.Printf("    Snippet: %s\n", snippet)
					}
				}
			}
		}
	}
	fmt.Println()

	// Step 6: Test aggregator search
	fmt.Println("Step 6: Test aggregator search...")
	aggResp := invokeSkill(ctx, apiURL, "aggregator.search", map[string]any{
		"query":   "军训注意事项",
		"sources": []string{"rag"},
		"top_k":   5,
	})
	if aggResp != nil {
		fmt.Printf("Total: %v\n", aggResp["total"])
		fmt.Printf("Sources: %v\n", aggResp["sources_count"])
	}
	fmt.Println()

	fmt.Println("=== Test Complete ===")
}

func invokeSkill(ctx context.Context, apiURL, skillName string, input map[string]any) map[string]any {
	body, _ := json.Marshal(SkillInvokeRequest{Input: input})

	req, _ := http.NewRequestWithContext(ctx, "POST",
		apiURL+"/v1/skills/"+skillName+":invoke",
		bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		fmt.Printf("❌ Invoke %s failed: %v\n", skillName, err)
		return nil
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)

	// SSOT always returns 200, check ok field
	var result struct {
		Ok    bool           `json:"ok"`
		Data  map[string]any `json:"data,omitempty"`
		Error *struct {
			Code    string `json:"code"`
			Message string `json:"message"`
		} `json:"error,omitempty"`
	}
	json.Unmarshal(respBody, &result)

	if !result.Ok {
		if result.Error != nil {
			fmt.Printf("❌ %s: %s - %s\n", skillName, result.Error.Code, result.Error.Message)
		}
		return nil
	}

	return result.Data
}

func getJobStatus(ctx context.Context, apiURL, jobID string) map[string]any {
	req, _ := http.NewRequestWithContext(ctx, "GET",
		apiURL+"/v1/jobs/"+jobID, nil)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return map[string]any{"status": "error", "error": err.Error()}
	}
	defer resp.Body.Close()

	var result map[string]any
	json.NewDecoder(resp.Body).Decode(&result)
	return result
}
