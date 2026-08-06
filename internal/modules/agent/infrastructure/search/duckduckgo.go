package search

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// DuckDuckGoGateway implements Gateway using the free DuckDuckGo Instant Answer API.
type DuckDuckGoGateway struct {
	client *http.Client
}

// NewDuckDuckGoGateway creates a DuckDuckGoGateway.
func NewDuckDuckGoGateway() *DuckDuckGoGateway {
	return &DuckDuckGoGateway{client: &http.Client{Timeout: 10 * time.Second}}
}

type ddgResponse struct {
	Abstract     string `json:"Abstract"`
	AbstractURL  string `json:"AbstractURL"`
	Heading      string `json:"Heading"`
	RelatedTopics []struct {
		Text     string `json:"Text"`
		FirstURL string `json:"FirstURL"`
	} `json:"RelatedTopics"`
}

func (g *DuckDuckGoGateway) Search(ctx context.Context, query string) ([]Result, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet,
		"https://api.duckduckgo.com/?format=json&q="+url.QueryEscape(query), nil)
	if err != nil {
		return nil, fmt.Errorf("create search request: %w", err)
	}

	resp, err := g.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("search request: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, 256*1024))
	if err != nil {
		return nil, fmt.Errorf("read search response: %w", err)
	}

	var ddg ddgResponse
	if err := json.Unmarshal(body, &ddg); err != nil {
		return nil, fmt.Errorf("parse search response: %w", err)
	}

	var results []Result
	if ddg.Abstract != "" {
		results = append(results, Result{
			Title:   ddg.Heading,
			URL:     ddg.AbstractURL,
			Snippet: ddg.Abstract,
		})
	}
	for _, topic := range ddg.RelatedTopics {
		if topic.Text != "" {
			results = append(results, Result{
				Title:   extractTitle(topic.Text),
				URL:     topic.FirstURL,
				Snippet: topic.Text,
			})
		}
	}

	return results, nil
}

func extractTitle(text string) string {
	parts := strings.SplitN(text, " - ", 2)
	if len(parts) == 2 {
		return parts[0]
	}
	return text
}
