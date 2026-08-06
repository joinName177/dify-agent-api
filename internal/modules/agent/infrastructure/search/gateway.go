// Package search provides web search capabilities for the agent.
package search

import "context"

// Result holds a single search result.
type Result struct {
	Title   string
	URL     string
	Snippet string
}

// Gateway is the port for performing web searches.
type Gateway interface {
	Search(ctx context.Context, query string) ([]Result, error)
}
