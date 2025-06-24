package main

import (
	"log"

	"github.com/abinrajmk8/wikigraph/shared"
)

func main() {
	graph := shared.NewGraph()

	graph.AddEdge("Albert Einstein", "Physics")
	graph.AddEdge("Albert Einstein", "Nobel Prize")
	graph.AddEdge("Physics", "Science")
	graph.AddEdge("Science", "Knowledge")
	graph.AddEdge("Nobel Prize", "Awards")

	err := graph.SaveToFile("data/graph.json")
	if err != nil {
		log.Fatal("[x] Failed to save graph : ", err)
	}
	log.Println("[+] Graph saved to data/graph.json")
}
