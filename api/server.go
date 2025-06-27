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

	//neo4j query
	ctx := context.Background()
	session := driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeRead})
	defer session.Close(ctx)

	relation := matchIntentToRelation(intent)
	log.Printf("Checking relation: %s against intent: %s", relation, intent)
	if relation == "" {
		http.Error(w, "No matching relation found for intent", http.StatusNotFound)
		return
	}

	// generate cypher query based on intent
	cypher, params := buildCypher(relation, entity)
	log.Printf("Running Cypher: %s with name = %s\n", cypher, entity)
	result, err := session.Run(ctx, cypher, params)

	if err != nil {
		http.Error(w, "Query failed: "+err.Error(), http.StatusInternalServerError)
		return
	}

	if result.Next(ctx) {
		record := result.Record()
		response := map[string]interface{}{}

		for _, key := range record.Keys {
			val, _ := record.Get(key)

			// Clean up array values
			if rawList, ok := val.([]interface{}); ok {
				var cleaned []string
				for _, item := range rawList {
					/* str := fmt.Sprint(item)
					str = strings.Trim(str, `"`)
					str = strings.TrimSpace(str)
					cleaned = append(cleaned, str) */
					cleaned = append(cleaned, strings.TrimSpace(fmt.Sprint(item)))
				}
				response[key] = cleaned
			} else {
				response[key] = val
			}
		}

		json.NewEncoder(w).Encode(response)
	} else {
		// Try suggesting a closest match
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

	intentKeywords := []string{"born", "died", "award", "prize", "receive", "won", "work", "field", "known", "nationality", "occupation"}

	queryLower := strings.ToLower(query)
	for _, keyword := range intentKeywords {
		if strings.Contains(queryLower, keyword) {
			return entity, keyword
		}
	}

	return entity, ""
}

func matchIntentToRelation(intent string) string {
	relations := []string{"BornIn", "DiedIn", "Field", "KnightedBy", "KnownFor", "Nationality", "Occupation", "WonAward", "WorkedAt"}

	intent = strings.ToLower(intent)
	for _, rel := range relations {
		prefix := strings.ToLower(splitCamelCase(rel)[0])
		if prefix == intent {
			return rel
		}
	}
	return ""
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
	query := fmt.Sprintf(`
		MATCH (a) WHERE toLower(a.name) = toLower($name)
		MATCH (a)-[:%s]->(b)
		RETURN collect(b.name) AS related`, relation)
	return query, map[string]interface{}{"name": entity}
}

// Try suggesting the nearest matching node by name
func suggestAlternative(sess neo4j.SessionWithContext, ctx context.Context, name string) (string, error) {
	res, err := sess.Run(ctx,
		`MATCH (n) WHERE toLower(n.name) CONTAINS toLower($name)
		 RETURN n.name LIMIT 1`, map[string]interface{}{"name": name})
	if err != nil {
		return "", err
	}
	if res.Next(ctx) {
		n, _ := res.Record().Get("n.name")
		return fmt.Sprint(n), nil
	}
	return "", nil
}
