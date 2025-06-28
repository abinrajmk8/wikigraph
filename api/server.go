package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"strings"

	"github.com/abinrajmk8/wikigraph/shared"
	"github.com/abinrajmk8/wikigraph/synonyms"
	"github.com/gorilla/mux"
	"github.com/jdkato/prose/v2"
	"github.com/joho/godotenv"
	"github.com/neo4j/neo4j-go-driver/v5/neo4j"
)

var graph *shared.Graph
var driver neo4j.DriverWithContext

func main() {
	// Load .env file
	if er := godotenv.Load(); er != nil {
		log.Fatalf("Error loading .env file: %v", er)
	}

	uri := os.Getenv("NEO4J_URI")
	username := os.Getenv("NEO4J_USER")
	password := os.Getenv("NEO4J_PASSWORD")

	var err error
	ctx := context.Background()
	driver, err = neo4j.NewDriverWithContext(uri, neo4j.BasicAuth(username, password, ""))
	if err != nil {
		log.Fatalf("Failed to create driver: %v", err)
	}
	defer driver.Close(ctx)

	r := mux.NewRouter()
	r.HandleFunc("/query", queryHandler).Methods("GET")
	r.HandleFunc("/search", handleSearch).Methods("GET")

	log.Println("Server started at localhost: 8080")
	log.Fatal(http.ListenAndServe(":8080", r))
}

// Handles NLP queries
func queryHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	rawQuery := r.URL.Query().Get("q")
	if rawQuery == "" {
		http.Error(w, "Missing value for 'q'", http.StatusBadRequest)
		return
	}

	entity, intent := extractEntityAndIntent(rawQuery)
	log.Printf("Detected intent: '%s', entity: '%s'\n", intent, entity)

	if entity == "" || intent == "" {
		http.Error(w, "Could not extract entity or intent", http.StatusBadRequest)
		return
	}

	relations, err := matchIntentToRelations(intent)
	if err != nil || len(relations) == 0 {
		http.Error(w, "No matching relation found for intent", http.StatusNotFound)
		return
	}
	log.Printf("Checking relation: %s against intent: %s", relations, intent)

	// Query Neo4j
	ctx := context.Background()
	session := driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeRead})
	defer session.Close(ctx)

	response := make(map[string]interface{})
	found := false

	for _, relation := range relations {
		cypher, params := buildCypher(relation, entity)
		log.Printf("Running Cypher: %s with name = %s\n", cypher, entity)
		result, err := session.Run(ctx, cypher, params)
		if err != nil {
			continue
		}
		if result.Next(ctx) {
			record := result.Record()
			for _, key := range record.Keys {
				val, _ := record.Get(key)
				if rawList, ok := val.([]interface{}); ok {
					var cleaned []string
					for _, item := range rawList {
						cleaned = append(cleaned, strings.TrimSpace(fmt.Sprint(item)))
					}
					response[relation] = cleaned
				} else {
					response[relation] = val
				}
				found = true
			}
		}
	}

	if found {
		json.NewEncoder(w).Encode(response)
	} else {
		alt, err := suggestAlternative(session, ctx, entity)
		if err == nil && alt != "" {
			http.Error(w, fmt.Sprintf("No result found. Did you mean: %s?", alt), http.StatusNotFound)
		} else {
			http.Error(w, "No result found", http.StatusNotFound)
		}
	}
}

// Basic search by partial entity name
func handleSearch(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	query := r.URL.Query().Get("q")

	if query == "" {
		http.Error(w, "Missing value for 'q' ", http.StatusBadRequest)
		return
	}

	ctx := context.Background()
	session := driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeRead})
	defer session.Close(ctx)

	result, err := session.Run(ctx,
		`MATCH (n)
		WHERE toLower(n.name) CONTAINS toLower($query)
		RETURN n.name LIMIT 10`,
		map[string]interface{}{"query": query})

	if err != nil {
		http.Error(w, "Query failed: "+err.Error(), http.StatusInternalServerError)
		return
	}

	var matches []string
	for result.Next(ctx) {
		name, _ := result.Record().Get("n.name")
		matches = append(matches, strings.Trim(fmt.Sprint(name), `"`))
	}

	if len(matches) == 0 {
		http.Error(w, "No result found", http.StatusNotFound)
		return
	}

	json.NewEncoder(w).Encode(matches)
}

// Extracts entity and intent token from query using NLP and token filtering
func extractEntityAndIntent(query string) (string, string) {
	doc, err := prose.NewDocument(query)
	if err != nil {
		log.Println("Error parsing query:", err)
		return "", ""
	}

	// Extract entity
	var entity string
	entityWords := map[string]bool{}
	for _, ent := range doc.Entities() {
		if ent.Label == "PERSON" || ent.Label == "ORG" {
			entity = ent.Text
			for _, word := range strings.Fields(ent.Text) {
				entityWords[strings.ToLower(word)] = true
			}
			break
		}
	}

	// Remove stop words and entity words
	stopWords := map[string]bool{
		"what": true, "is": true, "did": true, "was": true,
		"where": true, "when": true, "who": true, "the": true,
		"a": true, "an": true, "in": true, "on": true, "at": true,
	}

	var filteredTokens []string
	for _, tok := range doc.Tokens() {
		word := strings.ToLower(tok.Text)
		if !stopWords[word] && !entityWords[word] {
			filteredTokens = append(filteredTokens, word)
		}
	}

	intent := detectIntentFromTokens(filteredTokens)
	return entity, intent
}

func detectIntentFromTokens(tokens []string) string {
	relations := []string{
		"BirthPlace", "BirthDate", "DiedIn", "DeathDate", "Field", "KnownFor",
		"Nationality", "Awarded", "WorkedAt", "StudiedAt", "AwardYear",
		"Employer", "DoctoralInstitution", "Theory", "Membership", "Citizenship",
	}

	// Step 1: Direct prefix match
	for _, token := range tokens {
		for _, rel := range relations {
			parts := splitCamelCase(rel)
			if len(parts) > 0 {
				prefix := strings.ToLower(parts[0])
				if prefix == token {
					return token
				}
			}
		}
	}

	// Step 2: Check relation prefix synonyms against tokens
	for _, rel := range relations {
		parts := splitCamelCase(rel)
		if len(parts) == 0 {
			continue
		}
		prefix := strings.ToLower(parts[0])

		syns, err := synonyms.GetSynonyms(prefix, 5)
		if err != nil {
			continue
		}

		for _, token := range tokens {
			for _, syn := range syns {
				if syn == token {
					return token
				}
			}
		}
	}

	return "generalinfo"
}

// Maps detected intent to relation(s)
func matchIntentToRelations(intent string) ([]string, error) {
	intent = strings.ToLower(intent)
	relations := []string{
		"BirthPlace", "BirthDate", "DiedIn", "DeathDate", "Field", "KnownFor",
		"Nationality", "Awarded", "WorkedAt", "StudiedAt", "AwardYear",
		"Employer", "DoctoralInstitution", "Theory", "Membership", "Citizenship",
	}

	var matched []string

	// Step 1: Direct prefix match
	for _, rel := range relations {
		parts := splitCamelCase(rel)
		if len(parts) > 0 {
			prefix := strings.ToLower(parts[0])
			if prefix == intent {
				matched = append(matched, rel)
			}
		}
	}
	if len(matched) > 0 {
		return matched, nil
	}

	// Step 2: Check synonym of intent against relation prefixes
	syns, err := synonyms.GetSynonyms(intent, 5)
	if err != nil {
		return nil, err
	}

	for _, rel := range relations {
		parts := splitCamelCase(rel)
		if len(parts) == 0 {
			continue
		}
		prefix := strings.ToLower(parts[0])
		for _, syn := range syns {
			if prefix == syn {
				matched = append(matched, rel)
			}
		}
	}

	if len(matched) > 0 {
		return matched, nil
	}

	return nil, nil
}

// Splits camelCase to parts like ["Born", "In"] -> ["born", "in"]
func splitCamelCase(s string) []string {
	var words []string
	current := ""
	for i, r := range s {
		if i > 0 && r >= 'A' && r <= 'Z' {
			words = append(words, strings.ToLower(current))
			current = string(r)
		} else {
			current += string(r)
		}
	}
	words = append(words, strings.ToLower(current))
	return words
}

// Builds Cypher query based on relation
func buildCypher(relation, entity string) (string, map[string]interface{}) {
	query := fmt.Sprintf(`MATCH (a) WHERE toLower(a.name) = toLower($name) MATCH (a)-[:%s]->(b) RETURN collect(b.name) AS related`, relation)
	return query, map[string]interface{}{"name": entity}
}

// Suggests nearest entity match
func suggestAlternative(sess neo4j.SessionWithContext, ctx context.Context, name string) (string, error) {
	res, err := sess.Run(ctx, `MATCH (n) WHERE toLower(n.name) CONTAINS toLower($name) RETURN n.name LIMIT 1`, map[string]interface{}{"name": name})
	if err != nil {
		return "", err
	}
	if res.Next(ctx) {
		n, _ := res.Record().Get("n.name")
		return fmt.Sprint(n), nil
	}
	return "", nil
}
