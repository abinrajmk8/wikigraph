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

	graph, err = shared.LoadFromFile("data/graph.json") //edit this line to the required filename

	if err != nil {
		log.Fatalf("Failed to load graph: %v", err) // automatically exists
	}

	r := mux.NewRouter() //initialise router

	r.HandleFunc("/query", queryHandler).Methods("GET")
	r.HandleFunc("/search", handleSearch).Methods("GET")

	//start server
	log.Println("Server started at localhost: 8080")
	log.Fatal(http.ListenAndServe(":8080", r))
	log.Printf("Graph loaded with %d nodes", len(graph.Nodes))

	fmt.Println("Server started at localhost: 8080")

}

func cleanQuery(q string) string {
	prefixes := []string{
		"who is", "what is", "who was", "what was",
		"what did", "what are", "what were", "which fields did", "which field did",
		"tell me about", "give me information on", "describe", "explain",
		"in which field did", "in which area did",
		"did", "has", "was", "is", "name", "list",
	}

	q = strings.ToLower(strings.TrimSpace(q))
	q = strings.TrimSuffix(q, "?")

	for _, prefix := range prefixes {
		if strings.HasPrefix(q, prefix) {
			q = strings.TrimPrefix(q, prefix)
			break
		}
	}

	return strings.TrimSpace(q)
}

// Handler for /q endpoint
func queryHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	query := r.URL.Query().Get("q")
	query = cleanQuery(query)
	log.Printf("Extracted entity: '%s'\n", query)

	//neo4j query
	ctx := context.Background()
	session := driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeRead})
	defer session.Close(ctx)

	result, err := session.Run(ctx,
		`MATCH (n)
		WHERE toLower(n.name) = toLower($name)
		OPTIONAL MATCH (n)-[:RELATED_TO]->(m)
        RETURN n.summary as summary, collect(m.name) as related`, // Cypher query to find the node and its related nodes
		map[string]interface{}{"name": query}) // Pass the name parameter to the query

	if err != nil {
		http.Error(w, "Query failed: "+err.Error(), http.StatusInternalServerError)
		return
	}

	if result.Next(ctx) {
		record := result.Record()
		rawSummary := record.Values[0]

		// Clean related results
		var relatedCleaned []string
		if rawList, ok := record.Values[1].([]interface{}); ok {
			for _, item := range rawList {
				str := fmt.Sprint(item)
				str = strings.Trim(str, `"`)
				str = strings.TrimSpace(str)
				relatedCleaned = append(relatedCleaned, str)
			}
		}

		response := map[string]interface{}{
			"summary": rawSummary,
			"related": relatedCleaned,
		}
		json.NewEncoder(w).Encode(response)
	} else {
		http.Error(w, "No result found", http.StatusNotFound) // If no results, return 404
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
