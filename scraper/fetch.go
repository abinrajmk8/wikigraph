package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/abinrajmk8/wikigraph/shared"
	"github.com/joho/godotenv"
)

/* ------------------------------------------------------------
   Configuration
   -------------
   Store secrets (e.g. GEMINI_API_KEY) in a `.env` file at the
   project root, e.g.
       GEMINI_API_KEY=xxxxxxxxxxxxxxxx
   The file is automatically loaded at startup via godotenv.
------------------------------------------------------------*/

const (
    seedPerson  = "Albert Einstein"
    wikiAPIURL  = "https://en.wikipedia.org/w/api.php"
    geminiURL   = "https://generativelanguage.googleapis.com/v1beta/models/gemini-2.0-flash:generateContent"
    maxKeywords = 15
)

// Filled in init() after .env is loaded.
var geminiAPIKey string

func init() {
    // Load .env (silently ignore if the file doesn't exist)
    if err := godotenv.Load(); err != nil && !os.IsNotExist(err) {
        log.Fatalf("error loading .env: %v", err)
    }
    geminiAPIKey = os.Getenv("GEMINI_API_KEY")
}

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
    req.Header.Set("User-Agent", "wikigraph-scraper/0.5")

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

type geminiReq struct {
    Contents []struct {
        Parts []struct {
            Text string `json:"text"`
        } `json:"parts"`
    } `json:"contents"`
}

type geminiResp struct {
    Candidates []struct {
        Content struct {
            Parts []struct {
                Text string `json:"text"`
            } `json:"parts"`
        } `json:"content"`
    } `json:"candidates"`
}

func extractRelationsWithGemini(text string) ([]string, error) {
    if geminiAPIKey == "" {
        return nil, fmt.Errorf("GEMINI_API_KEY not set (check .env)")
    }

    prompt := fmt.Sprintf(`From the text below, extract at most %d short keywords or phrases that represent the key topics and domains associated with the *person* %s.\nIgnore mentions related to places, objects, products, storms, or any non‑person use of the word.\nReturn ONLY a JSON array of strings, with no explanations, markdown, or extra text.\n\n---\n\n%s`, maxKeywords, seedPerson, text)

    payload := geminiReq{
        Contents: []struct {
            Parts []struct {
                Text string `json:"text"`
            } `json:"parts"`
        }{{Parts: []struct {
            Text string `json:"text"`
        }{{Text: prompt}}}},
    }

    data, _ := json.Marshal(payload)
    endpoint := fmt.Sprintf("%s?key=%s", geminiURL, geminiAPIKey)

    resp, err := http.Post(endpoint, "application/json", bytes.NewReader(data))
    if err != nil {
        return nil, err
    }
    defer resp.Body.Close()
    if resp.StatusCode != http.StatusOK {
        b, _ := io.ReadAll(resp.Body)
        return nil, fmt.Errorf("gemini status %d: %s", resp.StatusCode, b)
    }

    var g geminiResp
    if err := json.NewDecoder(resp.Body).Decode(&g); err != nil {
        return nil, err
    }

    if len(g.Candidates) == 0 || len(g.Candidates[0].Content.Parts) == 0 {
        return nil, fmt.Errorf("empty Gemini response")
    }

    raw := strings.TrimSpace(g.Candidates[0].Content.Parts[0].Text)
    if strings.HasPrefix(raw, "```") {
        raw = strings.TrimPrefix(raw, "```json")
        raw = strings.TrimPrefix(raw, "```")
        raw = strings.TrimSuffix(raw, "```")
        raw = strings.TrimSpace(raw)
    }

    var keywords []string
    if err := json.Unmarshal([]byte(raw), &keywords); err == nil {
        return uniqStrings(keywords), nil
    }

    tokens := strings.FieldsFunc(raw, func(r rune) bool { return r == ',' || r == '\n' || r == ';' })
    for _, t := range tokens {
        if trimmed := strings.TrimSpace(t); trimmed != "" {
            keywords = append(keywords, trimmed)
        }
    }
    return uniqStrings(keywords), nil
}

func uniqStrings(in []string) []string {
    seen := map[string]struct{}{}
    out := []string{}
    for _, s := range in {
        if _, exists := seen[s]; exists {
            continue
        }
        seen[s] = struct{}{}
        out = append(out, s)
    }
    return out
}

func main() {
    fmt.Printf("[+] Seed: %s\n", seedPerson)

    extract, err := fetchWikiExtract(seedPerson)
    if err != nil {
        log.Fatalf("wiki error: %v", err)
    }
    fmt.Printf("    Wikipedia extract length: %d chars\n", len(extract))

    relations, err := extractRelationsWithGemini(extract)
    if err != nil {
        log.Fatalf("gemini error: %v", err)
    }
    fmt.Printf("    Gemini keywords: %v\n", relations)

    graph := shared.NewGraph()
    for _, kw := range relations {
        graph.AddEdge(seedPerson, kw)
    }

    _, src, _, _ := runtime.Caller(0)
    root := filepath.Dir(filepath.Dir(src))
    outPath := filepath.Join(root, "data", "graph.json")

    if err := graph.SaveToFile(outPath); err != nil {
        log.Fatalf("cannot save graph: %v", err)
    }
    log.Printf("[✓] Graph saved to %s (nodes: %d)\n", outPath, len(graph.Nodes))
}
