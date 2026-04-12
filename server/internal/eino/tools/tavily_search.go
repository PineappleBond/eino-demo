package tools

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

// TavilySearchInput is the input schema for the tavily search tool.
type TavilySearchInput struct {
	Query             string `json:"query" jsonschema_description:"Search query"`
	MaxResults        int    `json:"max_results,omitempty" jsonschema_description:"Maximum number of results to return (default 5)"`
	SearchDepth       string `json:"search_depth,omitempty" jsonschema_description:"Search depth: 'basic' or 'advanced' (default 'basic')"`
	IncludeAnswer     bool   `json:"include_answer,omitempty" jsonschema_description:"Include an AI-generated answer summary (default false)"`
	IncludeRawContent bool   `json:"include_raw_content,omitempty" jsonschema_description:"Include full page content (default false)"`
}

// TavilyResultItem is a single search result.
type TavilyResultItem struct {
	Title   string  `json:"title"`
	URL     string  `json:"url"`
	Score   float64 `json:"score"`
	Content string  `json:"content"`
}

// TavilySearchOutput is the output schema for the tavily search tool.
type TavilySearchOutput struct {
	Query   string             `json:"query"`
	Results []TavilyResultItem `json:"results"`
	Answer  string             `json:"answer,omitempty"`
}

// TavilySearchTool searches the web using the Tavily API.
type TavilySearchTool struct {
	apiKey string
}

// NewTavilySearchTool creates a Tavily search tool.
// Returns nil if apiKey is empty.
func NewTavilySearchTool(apiKey string) *TavilySearchTool {
	if apiKey == "" {
		return nil
	}
	return &TavilySearchTool{apiKey: apiKey}
}

// Run performs a web search via the Tavily API.
// On failure, returns a <tool_error> wrapped string with nil error.
func (t *TavilySearchTool) Run(input TavilySearchInput) string {
	maxResults := input.MaxResults
	if maxResults <= 0 {
		maxResults = 5
	}

	searchDepth := input.SearchDepth
	if searchDepth == "" {
		searchDepth = "basic"
	}

	reqBody := map[string]any{
		"query":        input.Query,
		"max_results":  maxResults,
		"search_depth": searchDepth,
	}
	if input.IncludeAnswer {
		reqBody["include_answer"] = true
	}
	if input.IncludeRawContent {
		reqBody["include_raw_content"] = true
	}

	body, err := json.Marshal(reqBody)
	if err != nil {
		return fmt.Sprintf("<tool_error>failed to marshal request: %s</tool_error>", err)
	}

	req, err := http.NewRequest("POST", "https://api.tavily.com/search", bytes.NewReader(body))
	if err != nil {
		return fmt.Sprintf("<tool_error>failed to create request: %s</tool_error>", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+t.apiKey)

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Sprintf("<tool_error>request failed: %s</tool_error>", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		return fmt.Sprintf("<tool_error>tavily API error (status %d): %s</tool_error>", resp.StatusCode, string(respBody))
	}

	var tavilyResp struct {
		Query   string `json:"query"`
		Answer  string `json:"answer,omitempty"`
		Results []struct {
			Title   string  `json:"title"`
			URL     string  `json:"url"`
			Score   float64 `json:"score"`
			Content string  `json:"content"`
		} `json:"results"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&tavilyResp); err != nil {
		return fmt.Sprintf("<tool_error>failed to decode response: %s</tool_error>", err)
	}

	out := &TavilySearchOutput{
		Query:   tavilyResp.Query,
		Results: make([]TavilyResultItem, 0, len(tavilyResp.Results)),
		Answer:  tavilyResp.Answer,
	}
	for _, r := range tavilyResp.Results {
		out.Results = append(out.Results, TavilyResultItem{
			Title:   r.Title,
			URL:     r.URL,
			Score:   r.Score,
			Content: r.Content,
		})
	}

	result, err := json.Marshal(out)
	if err != nil {
		return fmt.Sprintf("<tool_error>failed to marshal response: %s</tool_error>", err)
	}
	return string(result)
}
