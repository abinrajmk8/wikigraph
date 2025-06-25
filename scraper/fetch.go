// scraper/fetch.go  (branch: feature/dynamic-scraper, final working version)
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/abinrajmk8/wikigraph/shared"
)

/* ------------------------------------------------------------
   Wikipedia response types
------------------------------------------------------------*/

// from /api/rest_v1/page/summary/{title}
type summaryResp struct {
	Titles struct {
		Canonical string `json:"canonical"` // e.g. Albert_Einstein
	} `json:"titles"`
}

// from action=query&prop=links
type linkResponse struct {
	Query struct {
		Pages map[string]struct {
			Links []struct {
				Title string `json:"title"` // e.g. Physics
			} `json:"links"`
		} `json:"pages"`
	} `json:"query"`
}

/* ------------------------------------------------------------
   Extract entities for a Wikipedia page
------------------------------------------------------------*/

func ExtractEntitiesFromWikipedia(title string) ([]string, error) {
	ctx := context.Background()

	/* 1️⃣  Resolve canonical title  */
	summaryURL := fmt.Sprintf(
		"https://en.wikipedia.org/api/rest_v1/page/summary/%s",
		url.PathEscape(title),
	)
	sumJSON, err := fetchJSON(ctx, summaryURL)
	if err != nil {
		return nil, err
	}
	var s summaryResp
	if err := json.Unmarshal(sumJSON, &s); err != nil {
		return nil, err
	}
	canonical := s.Titles.Canonical // "Albert_Einstein"

	/* 2️⃣  Fetch up-to-20 outgoing links */
	linksURL := fmt.Sprintf(
		"https://en.wikipedia.org/w/api.php?action=query&titles=%s&prop=links&format=json&pllimit=20",
		canonical,
	)
	linksJSON, err := fetchJSON(ctx, linksURL)
	if err != nil {
		return nil, err
	}
	var lr linkResponse
	if err := json.Unmarshal(linksJSON, &lr); err != nil {
		return nil, err
	}

	entities := []string{}
	for _, page := range lr.Query.Pages {
		for _, link := range page.Links {
			// skip non-article namespaces such as "File:", "Category:", etc.
			if strings.Contains(link.Title, ":") {
				continue
			}
			entities = append(entities, link.Title)
		}
	}
	return entities, nil
}

/*
	------------------------------------------------------------
	  Simple GET helper that returns body or error

------------------------------------------------------------
*/
func fetchJSON(ctx context.Context, url string) ([]byte, error) {
	req, _ := http.NewRequestWithContext(ctx, "GET", url, nil)
	req.Header.Set("User-Agent", "wikigraph-demo/0.1")
	req.Header.Set("Accept", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("status %d: %s", resp.StatusCode, b)
	}
	return io.ReadAll(resp.Body)
}

/*
	------------------------------------------------------------
	  Main: build graph for one seed page

------------------------------------------------------------
*/
func main() {
	seed := "Albert Einstein" // change or loop over more topics
	fmt.Printf("[+] Fetching related pages for %q…\n", seed)

	related, err := ExtractEntitiesFromWikipedia(seed)
	if err != nil {
		log.Fatal("[x] Wikipedia fetch error:", err)
	}

	graph := shared.NewGraph()
	for _, rel := range related {
		graph.AddEdge(seed, rel)
	}

	// save to project-root/data/graph.json (works from any dir)
	_, src, _, _ := runtime.Caller(0)
	root := filepath.Dir(filepath.Dir(src))
	out := filepath.Join(root, "data", "graph.json")

	if err := graph.SaveToFile(out); err != nil {
		log.Fatal("[x] Failed to save graph:", err)
	}
	log.Printf("[✓] Graph saved to %s (%d nodes)\n", out, len(graph.Nodes))
}
