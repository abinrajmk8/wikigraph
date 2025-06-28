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
   fetch.go — fixed‑predicate ingest with in‑batch de‑duplication
   ------------------------------------------------------------*/

const (
	defaultSeed   = "Albert Einstein"
	wikiAPIURL    = "https://en.wikipedia.org/w/api.php"
	geminiURL     = "https://generativelanguage.googleapis.com/v1beta/models/gemini-2.0-flash:generateContent"
	predicateFile = "shared/predicates.json"
)

var (
	geminiAPIKey      string
	neo4jURI          string
	neo4jUser         string
	neo4jPassword     string
	allowedPredicates map[string]struct{}
)

func init() {
	_ = godotenv.Load()
	geminiAPIKey = os.Getenv("GEMINI_API_KEY")
	neo4jURI = os.Getenv("NEO4J_URI")
	neo4jUser = os.Getenv("NEO4J_USER")
	neo4jPassword = os.Getenv("NEO4J_PASSWORD")

	for k, v := range map[string]string{
		"GEMINI_API_KEY": geminiAPIKey,
		"NEO4J_URI":      neo4jURI,
		"NEO4J_USER":     neo4jUser,
		"NEO4J_PASSWORD": neo4jPassword,
	} {
		if v == "" {
			log.Fatalf("env var %s missing", k)
		}
	}
	loadPredicates()
}

/* ---------- predicate list ---------- */

func loadPredicates() {
	data, err := os.ReadFile(predicateFile)
	if err != nil {
		log.Println("warning: using default predicates")
		data = []byte(`[
			"BirthPlace","BirthDate","DeathPlace","DeathDate",
			"Field","KnownFor","Awarded","AwardYear",
			"StudiedAt","WorkedAt","Employer","Citizenship",
			"Nationality","Theory","Invention","Membership",
			"Influenced","Mentor","Student"
		]`)
	}
	var preds []string
	if err := json.Unmarshal(data, &preds); err != nil {
		log.Fatalf("invalid predicates file: %v", err)
	}
	allowedPredicates = make(map[string]struct{}, len(preds))
	for _, p := range preds {
		allowedPredicates[p] = struct{}{}
	}
}

/* ---------- data types ---------- */

type triple struct {
	Predicate  string `json:"predicate"`
	Object     string `json:"object"`
	ObjectType string `json:"object_type"`
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

/* ---------- helpers ---------- */

func seedFromCLI() string {
	if len(os.Args) > 1 {
		return strings.Join(os.Args[1:], " ")
	}
	return defaultSeed
}

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
	req.Header.Set("User-Agent", "wikigraph-triples/1.3")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("wiki %d: %s", resp.StatusCode, b)
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
		txt := strings.NewReplacer("\n", " ", "/", " ", "=", " ").Replace(pg.Extract)
		return strings.TrimSpace(txt), nil
	}
	return "", fmt.Errorf("no extract for %s", title)
}

func callGemini(prompt string) (string, error) {
	body, _ := json.Marshal(
		gemReq{Contents: []struct {
			Parts []struct {
				Text string `json:"text"`
			} `json:"parts"`
		}{
			{Parts: []struct {
				Text string `json:"text"`
			}{{Text: prompt}}},
		}},
	)
	resp, err := http.Post(fmt.Sprintf("%s?key=%s", geminiURL, geminiAPIKey),
		"application/json", bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("gemini %d: %s", resp.StatusCode, b)
	}

	var g gemResp
	if err := json.NewDecoder(resp.Body).Decode(&g); err != nil {
		return "", err
	}
	if len(g.Candidates) == 0 {
		return "", fmt.Errorf("empty Gemini reply")
	}
	return strings.TrimSpace(g.Candidates[0].Content.Parts[0].Text), nil
}

func extractTriples(seed, text string) ([]triple, error) {
	prompt := fmt.Sprintf(
		`Extract factual triples about %s. Return ONLY a JSON array with "predicate","object","object_type". Only use these predicates: %s

---
%s`, seed, strings.Join(getAllowedPredicates(), ", "), text)

	raw, err := callGemini(prompt)
	if err != nil {
		return nil, err
	}

	block := regexp.MustCompile(`(?s)\[\s*\{.*?\}\s*\]`).FindString(raw)
	if block == "" {
		block = raw
	}
	var all []triple
	if err := json.Unmarshal([]byte(block), &all); err != nil {
		return nil, fmt.Errorf("triples parse: %v", err)
	}

	var filtered []triple
	for _, t := range all {
		if _, ok := allowedPredicates[t.Predicate]; ok {
			filtered = append(filtered, t)
		}
	}
	return filtered, nil
}

func getAllowedPredicates() []string {
	out := make([]string, 0, len(allowedPredicates))
	for p := range allowedPredicates {
		out = append(out, p)
	}
	return out
}

/* ---------- string utils ---------- */

var reUnsafe = regexp.MustCompile(`[^A-Za-z0-9_]`)

func safeLabel(s string) string {
	if s == "" {
		return "Entity"
	}
	return strings.Title(reUnsafe.ReplaceAllString(s, ""))
}
func relType(p string) string {
	if p == "" {
		return "RELATED_TO"
	}
	return reUnsafe.ReplaceAllString(p, "_")
}
func norm(s string) string {
	return strings.ToLower(strings.Join(strings.Fields(strings.TrimSpace(s)), " "))
}

/* ---------- ingest ---------- */

func ingest(seed string, triples []triple, summary string) error {
	ctx := context.Background()
	drv, err := neo4j.NewDriverWithContext(neo4jURI, neo4j.BasicAuth(neo4jUser, neo4jPassword, ""))
	if err != nil {
		return err
	}
	defer drv.Close(ctx)

	sess := drv.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeWrite})
	defer sess.Close(ctx)

	// ensure Person node
	if _, err = sess.ExecuteWrite(ctx, func(tx neo4j.ManagedTransaction) (any, error) {
		_, e := tx.Run(ctx,
			`MERGE (p:Person {name:$n}) ON CREATE SET p.summary=$s`,
			map[string]any{"n": seed, "s": summary},
		)
		return nil, e
	}); err != nil {
		return err
	}

	seen := map[string]struct{}{}
	for _, tr := range triples {
		rel := relType(tr.Predicate)
		objLabel := safeLabel(tr.ObjectType)
		normObj := norm(tr.Object)

		key := strings.ToLower(rel) + "::" + normObj
		if _, dup := seen[key]; dup {
			continue
		}
		seen[key] = struct{}{}

		if _, err = sess.ExecuteWrite(ctx, func(tx neo4j.ManagedTransaction) (any, error) {
			cy := fmt.Sprintf(`
				MERGE (p:Person {name:$seed})
				MERGE (o:%s {norm:$norm})
				ON CREATE SET o.name=$obj
				MERGE (p)-[:%s]->(o)`, objLabel, rel)
			_, e := tx.Run(ctx, cy, map[string]any{
				"seed": seed,
				"norm": normObj,
				"obj":  tr.Object,
			})
			return nil, e
		}); err != nil {
			return err
		}
	}
	return nil
}

/* ---------- main ---------- */

func main() {
	seed := seedFromCLI()
	fmt.Println("[+] Ingest", seed)

	txt, err := fetchExtract(seed)
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println("    chars:", len(txt))

	triples, err := extractTriples(seed, txt)
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println("    triples:", len(triples))

	if err := ingest(seed, triples, txt); err != nil {
		log.Fatal(err)
	}
	fmt.Println("✓ Neo4j done")
}
