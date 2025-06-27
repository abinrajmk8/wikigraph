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
	"regexp"
	"strings"

	"github.com/joho/godotenv"
	"github.com/neo4j/neo4j-go-driver/v5/neo4j"
)

/* ------------------------------------------------------------
   fetch.go — scrape, extract triples, ingest into Neo4j
   ------------------------------------------------------------*/

const (
	seedPerson = "Albert Einstein"
	wikiAPIURL = "https://en.wikipedia.org/w/api.php"
	geminiURL  = "https://generativelanguage.googleapis.com/v1beta/models/gemini-2.0-flash:generateContent"
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

/* ---------- STEP 1: fetch raw article text ---------- */

func fetchExtract(title string) (string, error) {
	req, _ := http.NewRequest("GET", wikiAPIURL, nil)
	q := req.URL.Query()
	q.Set("action", "query")
	q.Set("prop", "extracts")
	q.Set("explaintext", "1")
	q.Set("redirects", "1")
	q.Set("titles", title)
	q.Set("format", "json")
	req.URL.RawQuery = q.Encode()
	req.Header.Set("User-Agent", "wikigraph-triples/1.1")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("wiki status %d: %s", resp.StatusCode, b)
	}

	var res struct {
		Query struct {
			Pages map[string]struct {
				Extract string `json:"extract"`
			} `json:"pages"`
		} `json:"query"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&res); err != nil {
		return "", err
	}
	for _, pg := range res.Query.Pages {
		cleaned := strings.ReplaceAll(pg.Extract, "\n", " ")
		cleaned = strings.ReplaceAll(cleaned, "/", " ")
		cleaned = strings.ReplaceAll(cleaned, "=", " ")
		return strings.TrimSpace(cleaned), nil
	}
	return "", fmt.Errorf("no extract found")
}

/* ---------- STEP 2: LLM → triples ---------- */

type triple struct {
	Predicate string `json:"predicate"`
	Object    string `json:"object"`
}

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

func extractTriples(text string) ([]triple, error) {
	if geminiAPIKey == "" {
		return nil, fmt.Errorf("GEMINI_API_KEY missing")
	}

	prompt := `Extract key factual triples about the person ` + seedPerson + ` from the text below.
Return **ONLY** a JSON array of objects with exact keys "predicate" and "object". 
Use concise, CamelCase predicates such as "BornIn", "WonAward", "KnownFor", "DiedIn", "Field", "Nationality".` + "\n\n---\n\n" + text

	req := gemReq{Contents: []struct {
		Parts []struct {
			Text string `json:"text"`
		} `json:"parts"`
	}{{Parts: []struct {
		Text string `json:"text"`
	}{{Text: prompt}}}}}
	data, _ := json.Marshal(req)

	resp, err := http.Post(fmt.Sprintf("%s?key=%s", geminiURL, geminiAPIKey), "application/json", bytes.NewReader(data))
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("gemini %d: %s", resp.StatusCode, b)
	}

	var g gemResp
	if err := json.NewDecoder(resp.Body).Decode(&g); err != nil {
		return nil, err
	}
	if len(g.Candidates) == 0 {
		return nil, fmt.Errorf("empty response")
	}

	raw := strings.TrimSpace(g.Candidates[0].Content.Parts[0].Text)
	raw = strings.Trim(raw, "`\n ")
	raw = strings.TrimPrefix(raw, "json")
	raw = strings.TrimSpace(raw)

	var triples []triple
	if err := json.Unmarshal([]byte(raw), &triples); err != nil {
		return nil, fmt.Errorf("parse triples: %v\nraw: %s", err, raw)
	}
	return triples, nil
}

/* ---------- STEP 3: ingest triples ---------- */

var reUnsafe = regexp.MustCompile(`[^A-Za-z0-9_]`)

func relType(pred string) string {
	s := reUnsafe.ReplaceAllString(pred, "_")
	if s == "" {
		s = "RELATED"
	}
	return s
}

func norm(s string) string {
	return strings.ToLower(strings.Join(strings.Fields(strings.TrimSpace(s)), " "))
}

func ingest(triples []triple, summary string) error {
	ctx := context.Background()
	drv, err := neo4j.NewDriverWithContext(neo4jURI, neo4j.BasicAuth(neo4jUser, neo4jPassword, ""))
	if err != nil {
		return err
	}
	defer drv.Close(ctx)

	sess := drv.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeWrite})
	defer sess.Close(ctx)

	_, err = sess.ExecuteWrite(ctx, func(tx neo4j.ManagedTransaction) (any, error) {
		_, e := tx.Run(ctx, `MERGE (p:Person {name:$name}) ON CREATE SET p.summary=$sum`, map[string]any{"name": seedPerson, "sum": summary})
		return nil, e
	})
	if err != nil {
		return err
	}

	seen := make(map[string]bool)

	for _, tr := range triples {
		rel := relType(tr.Predicate)
		objNorm := norm(tr.Object)
		key := rel + "::" + objNorm
		if seen[key] {
			fmt.Printf("duplicate found %s::%s skipping\n", rel, tr.Object)
			continue
		}
		seen[key] = true

		_, err = sess.ExecuteWrite(ctx, func(tx neo4j.ManagedTransaction) (any, error) {
			cypher := fmt.Sprintf(`MERGE (p:Person {name:$name}) MERGE (o:Entity {norm:$norm}) ON CREATE SET o.name=$obj MERGE (p)-[:%s]->(o)`, rel)
			_, e := tx.Run(ctx, cypher, map[string]any{
				"name": seedPerson,
				"obj":  tr.Object,
				"norm": norm(tr.Object),
			})
			return nil, e
		})

		if err != nil {
			return err
		}
	}
	return nil
}

/* ---------- MAIN ---------- */

func main() {
	fmt.Println("[+] Ingesting", seedPerson)

	text, err := fetchExtract(seedPerson)
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println("    extract chars:", len(text))

	triples, err := extractTriples(text)
	if err != nil {
		log.Fatal(err)
	}
	fmt.Printf("    triples: %v\n", triples)

	if err := ingest(triples, text); err != nil {
		log.Fatal(err)
	}
	fmt.Println(" written to Neo4j")
}
