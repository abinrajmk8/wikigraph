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
	"time"

	"github.com/joho/godotenv"
	"github.com/neo4j/neo4j-go-driver/v5/neo4j"
)

/* ------------------------------------------------------------
   fetch.go — full ingest with automatic Predicate alias creation
   ------------------------------------------------------------*/

const (
	defaultSeed = "Albert Einstein"
	wikiAPIURL  = "https://en.wikipedia.org/w/api.php"
	geminiURL   = "https://generativelanguage.googleapis.com/v1beta/models/gemini-2.0-flash:generateContent"
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
	for k, v := range map[string]string{"GEMINI_API_KEY": geminiAPIKey, "NEO4J_URI": neo4jURI, "NEO4J_USER": neo4jUser, "NEO4J_PASSWORD": neo4jPassword} {
		if v == "" {
			log.Fatalf("env var %s missing", k)
		}
	}
}

/* ---------- CLI seed ---------- */
func seedFromCLI() string {
	if len(os.Args) > 1 {
		return strings.Join(os.Args[1:], " ")
	}
	return defaultSeed
}

/* ---------- Wikipedia extract ---------- */
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
		txt := strings.ReplaceAll(pg.Extract, "\n", " ")
		txt = strings.ReplaceAll(txt, "/", " ")
		txt = strings.ReplaceAll(txt, "=", " ")
		return strings.TrimSpace(txt), nil
	}
	return "", fmt.Errorf("no extract for %s", title)
}

/* ---------- LLM: triples ---------- */
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

func callGemini(prompt string) (string, error) {
	body, _ := json.Marshal(gemReq{Contents: []struct {
		Parts []struct {
			Text string `json:"text"`
		} `json:"parts"`
	}{{Parts: []struct {
		Text string `json:"text"`
	}{{Text: prompt}}}}})

	resp, err := http.Post(fmt.Sprintf("%s?key=%s", geminiURL, geminiAPIKey), "application/json", bytes.NewReader(body))
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
	prompt := fmt.Sprintf(`Extract factual triples about %s. Return ONLY a JSON array with "predicate","object","object_type" (Award, Institution, Field, Place, Person, Event, Nationality, Theory, Concept). Use CamelCase predicates.\n\n---\n%s`, seed, text)
	raw, err := callGemini(prompt)
	if err != nil {
		return nil, err
	}
	reJSON := regexp.MustCompile(`(?s)\[\s*\{.*?\}\s*\]`)
	block := reJSON.FindString(raw)
	if block == "" {
		block = raw
	}
	var out []triple
	if err := json.Unmarshal([]byte(block), &out); err != nil {
		return nil, fmt.Errorf("triples parse: %v", err)
	}
	return out, nil
}

/* ---------- Predicate alias creation ---------- */
func ensurePredicateWithAliases(ctx context.Context, sess neo4j.SessionWithContext, label string) error {
	// 1. check existence
	exists, err := sess.ExecuteRead(ctx, func(tx neo4j.ManagedTransaction) (any, error) {
		res, e := tx.Run(ctx, `MATCH (p:Predicate {name:$n}) RETURN p LIMIT 1`, map[string]any{"n": label})
		if e != nil {
			return false, e
		}
		return res.Next(ctx), nil
	})
	if err != nil {
		return err
	}
	if exists.(bool) {
		return nil // already present
	}

	// 2. fetch 4–5 aliases via Gemini
	time.Sleep(5 * time.Second)

	prompt := fmt.Sprintf(`Give 4 to 5 common English synonyms or phrases a user might use to indicate the relationship label "%s" between a person and another entity. Return ONLY a JSON array of strings.`, label)
	raw, err := callGemini(prompt)
	if err != nil {
		return err
	}
	var aliases []string
	if err := json.Unmarshal([]byte(raw), &aliases); err != nil {
		// fallback: split by commas/newlines
		for _, tok := range strings.FieldsFunc(raw, func(r rune) bool { return r == ',' || r == '\n' }) {
			if s := strings.TrimSpace(tok); s != "" {
				aliases = append(aliases, s)
			}
		}
	}
	if len(aliases) == 0 {
		aliases = []string{strings.ToLower(label)}
	}

	// 3. create the node
	_, err = sess.ExecuteWrite(ctx, func(tx neo4j.ManagedTransaction) (any, error) {
		_, e := tx.Run(ctx, `CREATE (p:Predicate {name:$n, aliases:$a})`, map[string]any{"n": label, "a": aliases})
		return nil, e
	})
	return err
}

/* ---------- Ingest ---------- */
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

func ingest(seed string, triples []triple, summary string) error {
	ctx := context.Background()
	drv, err := neo4j.NewDriverWithContext(neo4jURI, neo4j.BasicAuth(neo4jUser, neo4jPassword, ""))
	if err != nil {
		return err
	}
	defer drv.Close(ctx)

	sess := drv.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeWrite})
	defer sess.Close(ctx)

	_, err = sess.ExecuteWrite(ctx, func(tx neo4j.ManagedTransaction) (any, error) {
		_, e := tx.Run(ctx, `MERGE (p:Person {name:$n}) ON CREATE SET p.summary=$s`, map[string]any{"n": seed, "s": summary})
		return nil, e
	})
	if err != nil {
		return err
	}

	seen := make(map[string]struct{})
	for _, tr := range triples {
		relLabel := relType(tr.Predicate)
		if err := ensurePredicateWithAliases(ctx, sess, relLabel); err != nil {
			log.Println("predicate ensure error:", err)
		}
		objLabel := safeLabel(tr.ObjectType)
		key := relLabel + "::" + norm(tr.Object)
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}

		_, err = sess.ExecuteWrite(ctx, func(tx neo4j.ManagedTransaction) (any, error) {
			cy := fmt.Sprintf(`MERGE (p:Person {name:$seed}) MERGE (o:%s {norm:$norm}) ON CREATE SET o.name=$obj MERGE (p)-[:%s]->(o)`, objLabel, relLabel)
			_, e := tx.Run(ctx, cy, map[string]any{"seed": seed, "norm": norm(tr.Object), "obj": tr.Object})
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
	fmt.Println("    triples", len(triples))

	if err := ingest(seed, triples, txt); err != nil {
		log.Fatal(err)
	}
	fmt.Println("✓ Neo4j done")
}
