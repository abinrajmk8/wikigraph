package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"strings"

	"github.com/joho/godotenv"
	"github.com/neo4j/neo4j-go-driver/v5/neo4j"
)

/* ------------------------------------------------------------
   fetch.go — scrape, extract, ingest directly into Neo4j
   --------------------------------------------------------
   • Uses .env for GEMINI_API_KEY and NEO4J_* creds
   • No more graph.json; data is persisted in Aura
------------------------------------------------------------*/

const (
	seedPerson  = "Albert Einstein"
	wikiAPIURL  = "https://en.wikipedia.org/w/api.php"
	geminiURL   = "https://generativelanguage.googleapis.com/v1beta/models/gemini-2.0-flash:generateContent"
	maxKeywords = 15
)

var (
	geminiAPIKey  string
	neo4jURI      string
	neo4jUser     string
	neo4jPassword string
)

func init() {
	_ = godotenv.Load()
	geminiAPIKey = os.Getenv("GEMINI_API_KEY")
	neo4jURI = os.Getenv("NEO4J_URI")
	neo4jUser = os.Getenv("NEO4J_USER")
	neo4jPassword = os.Getenv("NEO4J_PASSWORD")
}

/* ---------------- Wikipedia helper ---------------- */

func fetchWikiExtract(title string) (string, error) {
	req, _ := http.NewRequest("GET", wikiAPIURL, nil)
	q := req.URL.Query()
	q.Set("action", "query")
	q.Set("prop", "extracts")
	q.Set("explaintext", "1")
	q.Set("redirects", "1")
	q.Set("titles", title)
	q.Set("format", "json")
	req.URL.RawQuery = q.Encode()
	req.Header.Set("User-Agent", "wikigraph-ingest/1.1")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("wiki status %d: %s", resp.StatusCode, b)
	}

	var wiki struct {
		Query struct {
			Pages map[string]struct {
				Extract string `json:"extract"`
			} `json:"pages"`
		} `json:"query"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&wiki); err != nil {
		return "", err
	}
	for _, pg := range wiki.Query.Pages {
		return pg.Extract, nil
	}
	return "", fmt.Errorf("no extract found for %s", title)
}

/* ---------------- Gemini helper ---------------- */

type gemReq struct {
	Contents []struct {
		Parts []struct {
			Text string `json:"text"`
		} `json:"parts"`
	} `json:"contents"`
}

type gemResp struct {
	Candidates []struct {
		Content struct {
			Parts []struct {
				Text string `json:"text"`
			} `json:"parts"`
		} `json:"content"`
	} `json:"candidates"`
}

func extractRelations(text string) ([]string, error) {
	if geminiAPIKey == "" {
		return nil, fmt.Errorf("GEMINI_API_KEY not set")
	}

	prompt := fmt.Sprintf(`From the text below, extract at most %d short keywords or phrases that represent the key topics and domains associated with the *person* %s.\nReturn ONLY a JSON array of strings.\n\n---\n\n%s`, maxKeywords, seedPerson, text)

	body, _ := json.Marshal(gemReq{Contents: []struct {
		Parts []struct {
			Text string `json:"text"`
		} `json:"parts"`
	}{{Parts: []struct {
		Text string `json:"text"`
	}{{Text: prompt}}}}})

	resp, err := http.Post(fmt.Sprintf("%s?key=%s", geminiURL, geminiAPIKey), "application/json", bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("gemini status %d: %s", resp.StatusCode, b)
	}

	var g gemResp
	if err := json.NewDecoder(resp.Body).Decode(&g); err != nil {
		return nil, err
	}

	if len(g.Candidates) == 0 {
		return nil, fmt.Errorf("empty Gemini response")
	}

	raw := strings.TrimSpace(g.Candidates[0].Content.Parts[0].Text)
	if strings.HasPrefix(raw, "```") {
		raw = strings.Trim(raw, "`\n ")
	}

	var arr []string
	if err := json.Unmarshal([]byte(raw), &arr); err == nil {
		return dedup(arr), nil
	}

	// Fallback naive split
	for _, tok := range strings.FieldsFunc(raw, func(r rune) bool { return r == ',' || r == '\n' }) {
		if s := strings.TrimSpace(tok); s != "" {
			arr = append(arr, s)
		}
	}
	return dedup(arr), nil
}

func dedup(in []string) []string {
	seen := map[string]struct{}{}
	out := []string{}
	for _, s := range in {
		if _, ok := seen[s]; ok {
			continue
		}
		seen[s] = struct{}{}
		out = append(out, s)
	}
	return out
}

/* ---------------- Neo4j ingest ---------------- */

func ingestNeo4j(relations []string) error {
	if neo4jURI == "" || neo4jUser == "" || neo4jPassword == "" {
		return fmt.Errorf("NEO4J credentials not set in .env")
	}

	ctx := context.Background()
	driver, err := neo4j.NewDriverWithContext(neo4jURI, neo4j.BasicAuth(neo4jUser, neo4jPassword, ""))
	if err != nil {
		return err
	}
	defer driver.Close(ctx)

	session := driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeWrite})
	defer session.Close(ctx)

	_, err = session.ExecuteWrite(ctx, func(tx neo4j.ManagedTransaction) (any, error) {
		_, e := tx.Run(ctx, `MERGE (p:Person {name:$name})`, map[string]any{"name": seedPerson})
		return nil, e
	})
	if err != nil {
		return err
	}

	for _, kw := range relations {
		_, err = session.ExecuteWrite(ctx, func(tx neo4j.ManagedTransaction) (any, error) {
			_, e := tx.Run(ctx, `MERGE (p:Person {name:$seed}) MERGE (t:Topic {name:$topic}) MERGE (p)-[:RELATED_TO]->(t)`, map[string]any{"seed": seedPerson, "topic": kw})
			return nil, e
		})
		if err != nil {
			return err
		}
	}
	return nil
}

/* ---------------- main ---------------- */

func main() {
	fmt.Printf("[+] Seed: %s\n", seedPerson)

	extract, err := fetchWikiExtract(seedPerson)
	if err != nil {
		log.Fatalf("wiki error: %v", err)
	}
	fmt.Printf("    Extract length: %d chars\n", len(extract))

	relations, err := extractRelations(extract)
	if err != nil {
		log.Fatalf("gemini error: %v", err)
	}
	fmt.Printf("    Gemini keywords: %v\n", relations)

	if err := ingestNeo4j(relations); err != nil {
		log.Fatalf("neo4j ingest error: %v", err)
	}
	fmt.Println("✅ Data written to Neo4j!")
}
