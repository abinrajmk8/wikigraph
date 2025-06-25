package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strings"

	"github.com/abinrajmk8/wikigraph/shared"

	"github.com/gorilla/mux"
)

var graph *shared.Graph

func main() {
	var err error
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

// Handler for /q endpoint
func queryHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	query := r.URL.Query().Get("q")

	node, exists := graph.Nodes[query]

	//if the entity does not exist in the graph, error is returned
	if !exists {
		http.Error(w, "Entity not found", http.StatusNotFound)
		return
	}

	//if the entity exists, return the node as JSON
	response := map[string]interface{}{
		"name":  node.Name,
		"edges": node.Edges,
	}

	json.NewEncoder(w).Encode(response) // Encode the response as JSON and write it to the response writer which is sent back to the client

}

func handleSearch(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	query := r.URL.Query().Get("q")

	if query == "" {
		http.Error(w, "Missing value for 'q' ", http.StatusBadRequest)
		return
	}

	var result []string

	for _, node := range graph.Nodes {
		if strings.Contains(strings.ToLower(node.Name), strings.ToLower(query)) {
			result = append(result, node.Name)
		}

		for _, edge := range node.Edges {
			if strings.Contains(strings.ToLower(edge), strings.ToLower(query)) {
				result = append(result, edge)
			}
		}
	}

	if len(result) == 0 {
		http.Error(w, "No result found", http.StatusNotFound)
		return
	}

	json.NewEncoder(w).Encode(result)
}
