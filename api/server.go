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

	// Initialize Neo4j driver
	er := godotenv.Load()
	if er != nil {
		log.Fatalf("Error loading .env file: %v", er) // automatically exits
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

	r := mux.NewRouter() //initialise router

	r.HandleFunc("/query", queryHandler).Methods("GET")
	r.HandleFunc("/search", handleSearch).Methods("GET")

	//start server
	log.Println("Server started at localhost: 8080")
	log.Fatal(http.ListenAndServe(":8080", r))
	log.Printf("Graph loaded with %d nodes", len(graph.Nodes))

	fmt.Println("Server started at localhost: 8080")

}

// Handler for /q endpoint
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

	//neo4j query
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

func handleSearch(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	query := r.URL.Query().Get("q")

	if query == "" {
		http.Error(w, "Missing value for 'q' ", http.StatusBadRequest)
		return
	}

	//neo4j query
	ctx := context.Background()
	session := driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeRead})
	defer session.Close(ctx)

	result, err := session.Run(ctx,
		`MATCH (n)
		WHERE toLower(n.name) CONTAINS toLower($query)
		RETURN n.name LIMIT 10`,
		map[string]interface{}{"query": query}) // Pass the query parameter to the Cypher query

	if err != nil {
		http.Error(w, "Query failed: "+err.Error(), http.StatusInternalServerError)
		return
	}

	var matches []string
	for result.Next(ctx) {
		name, _ := result.Record().Get("n.name")
		cleaned := strings.Trim(fmt.Sprint(name), `"`) // remove surrounding quotes
		matches = append(matches, cleaned)
	}

	if len(matches) == 0 {
		http.Error(w, "No result found", http.StatusNotFound)
		return
	}

	json.NewEncoder(w).Encode(matches)
}

func extractEntityAndIntent(query string) (string, string) {
	doc, err := prose.NewDocument(query)
	if err != nil {
		log.Println("Error parsing query:", err)
		return "", ""
	}
	var entity string
	for _, ent := range doc.Entities() {
		if ent.Label == "PERSON" || ent.Label == "ORG" {
			entity = ent.Text
			break
		}
	}
	intentKeywords := []string{"born", "died", "award", "prize", "receive", "won", "work", "field", "known", "nationality", "occupation", "citizen", "study"}
	queryLower := strings.ToLower(query)
	for _, keyword := range intentKeywords {
		if strings.Contains(queryLower, keyword) {
			return entity, keyword
		}
	}
	return entity, ""
}

func matchIntentToRelations(intent string) ([]string, error) {
	intentToRelations := map[string][]string{
		"award":       {"Awarded", "AwardYear"},
		"prize":       {"Awarded", "AwardYear"},
		"receive":     {"Awarded", "AwardYear"},
		"won":         {"Awarded", "AwardYear"},
		"born":        {"BirthDate", "BirthPlace"},
		"died":        {"DeathDate", "DiedIn"},
		"study":       {"StudiedAt"},
		"work":        {"WorkedAt", "Employer"},
		"field":       {"Field"},
		"known":       {"KnownFor"},
		"citizen":     {"Citizenship", "Nationality"},
		"nationality": {"Nationality"},
		"occupation":  {"Occupation"},
	}
	if rels, ok := intentToRelations[intent]; ok {
		return rels, nil
	}

	relations := []string{"BirthPlace", "BirthDate", "DiedIn", "DeathDate", "Field", "KnownFor", "Nationality", "Awarded", "WorkedAt", "StudiedAt", "AwardYear", "Employer", "DoctoralInstitution", "Theory", "Membership", "Citizenship"}
	intent = strings.ToLower(intent)
	for _, rel := range relations {
		prefix := strings.ToLower(splitCamelCase(rel)[0])
		if prefix == intent {
			return []string{rel}, nil
		}
	}

	ctx := context.Background()
	session := driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeRead})
	defer session.Close(ctx)
	res, err := session.Run(ctx, `
		MATCH (n)
		WHERE ANY(alias IN n.aliases WHERE toLower(alias) CONTAINS toLower($intent))
		RETURN n.name LIMIT 1`, map[string]interface{}{"intent": intent})
	if err != nil {
		return nil, err
	}
	if res.Next(ctx) {
		rel, _ := res.Record().Get("n.name")
		return []string{fmt.Sprint(rel)}, nil
	}

	syns, err := synonyms.GetSynonyms(intent, 5)
	if err != nil {
		return nil, nil
	}
	for _, syn := range syns {
		rels, err := matchIntentToRelations(syn)
		if err == nil && len(rels) > 0 {
			return rels, nil
		}
	}
	return nil, nil
}

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

func buildCypher(relation, entity string) (string, map[string]interface{}) {
	query := fmt.Sprintf(`MATCH (a) WHERE toLower(a.name) = toLower($name) MATCH (a)-[:%s]->(b) RETURN collect(b.name) AS related`, relation)
	return query, map[string]interface{}{"name": entity}
}

// Try suggesting the nearest matching node by name
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
