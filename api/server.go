package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"strings"

	"github.com/gorilla/mux"
	"github.com/jdkato/prose/v2"
	"github.com/joho/godotenv"
	"github.com/neo4j/neo4j-go-driver/v5/neo4j"
)

var driver neo4j.DriverWithContext

func main() {
	if err := godotenv.Load(); err != nil {
		log.Fatalf("Error loading .env file: %v", err)
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

	log.Println("Server started at localhost:8080")
	log.Fatal(http.ListenAndServe(":8080", r))
}

// Extracts entity using prose, and intent from external Python API
func extractEntityAndIntent(query string) (string, string) {
	doc, err := prose.NewDocument(query)
	if err != nil {
		log.Println("Error parsing query:", err)
		return "", ""
	}

	// Extract PERSON or ORG entity
	var entity string
	for _, ent := range doc.Entities() {
		if ent.Label == "PERSON" || ent.Label == "ORG" {
			entity = ent.Text
			break
		}
	}
	if entity == "" {
		log.Println("No entity found")
		return "", ""
	}

	// Get intent from Python model
	intent, err := getIntentFromPythonAPI(query)
	if err != nil {
		log.Println("Error calling Python API:", err)
		return entity, ""
	}

	return entity, intent
}

// Call external Python API and get intent as plain text
func getIntentFromPythonAPI(query string) (string, error) {
	// Prepare JSON body
	jsonData := map[string]string{"text": query}
	jsonBytes, err := json.Marshal(jsonData)
	if err != nil {
		return "", err
	}

	// Send POST request with JSON
	resp, err := http.Post(
		os.Getenv("URL"),
		"application/json",
		strings.NewReader(string(jsonBytes)),
	)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("Python API returned status: %d", resp.StatusCode)
	}

	// Parse JSON response
	var result struct {
		Intent string `json:"intent"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", err
	}

	return strings.TrimSpace(result.Intent), nil
}

// Query handler: receives query, extracts entity & intent, and returns Neo4j result
func queryHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	rawQuery := r.URL.Query().Get("q")
	if rawQuery == "" {
		http.Error(w, "Missing value for 'q'", http.StatusBadRequest)
		return
	}

	entity, intent := extractEntityAndIntent(rawQuery)
	log.Printf("Detected entity: '%s', intent: '%s'\n", entity, intent)

	if entity == "" || intent == "" {
		http.Error(w, "Could not detect entity or intent", http.StatusBadRequest)
		return
	}

	ctx := context.Background()
	session := driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeRead})
	defer session.Close(ctx)

	cypher, params := buildCypher(intent, entity)
	log.Printf("Running Cypher: %s with params: %v\n", cypher, params)

	result, err := session.Run(ctx, cypher, params)
	if err != nil {
		http.Error(w, "Query failed: "+err.Error(), http.StatusInternalServerError)
		return
	}

	response := make(map[string]interface{})
	found := false

	for result.Next(ctx) {
		record := result.Record()
		val, _ := record.Get("related")
		response[intent] = val
		found = true
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

// Build Cypher query from entity and intent
func buildCypher(intent, entity string) (string, map[string]interface{}) {
	query := fmt.Sprintf(`
		MATCH (a)
		WHERE toLower(a.name) = toLower($name)
		MATCH (a)-[:%s]->(b)
		RETURN collect(b.name) AS related
	`, intent)
	return query, map[string]interface{}{"name": entity}
}

// Fallback suggestion for similar entity
func suggestAlternative(sess neo4j.SessionWithContext, ctx context.Context, name string) (string, error) {
	res, err := sess.Run(ctx, `
		MATCH (n) WHERE toLower(n.name) CONTAINS toLower($name) 
		RETURN n.name LIMIT 1
	`, map[string]interface{}{"name": name})
	if err != nil {
		return "", err
	}
	if res.Next(ctx) {
		val, _ := res.Record().Get("n.name")
		return fmt.Sprint(val), nil
	}
	return "", nil
}

// Entity search for autocomplete or suggestions
func handleSearch(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	query := r.URL.Query().Get("q")
	if query == "" {
		http.Error(w, "Missing value for 'q'", http.StatusBadRequest)
		return
	}

	ctx := context.Background()
	session := driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeRead})
	defer session.Close(ctx)

	result, err := session.Run(ctx, `
		MATCH (n) 
		WHERE toLower(n.name) CONTAINS toLower($query) 
		RETURN n.name LIMIT 10
	`, map[string]interface{}{"query": query})

	if err != nil {
		http.Error(w, "Query failed: "+err.Error(), http.StatusInternalServerError)
		return
	}

	var names []string
	for result.Next(ctx) {
		name, _ := result.Record().Get("n.name")
		names = append(names, fmt.Sprint(name))
	}

	if len(names) == 0 {
		http.Error(w, "No result found", http.StatusNotFound)
		return
	}

	json.NewEncoder(w).Encode(names)
}
