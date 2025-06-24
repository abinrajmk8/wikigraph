package shared

import (
	"encoding/json"
	"os"
)

// Node represents an entity in the knowledge graph.
type Node struct {
	Name  string   `json:"name"`
	Edges []string `json:"edges"`
}

// Graph represents the knowledge graph with nodes and edges.
type Graph struct {
	Nodes map[string]*Node `json:"nodes"`
}

// NewGraph creates an empty Graph.
func NewGraph() *Graph {
	return &Graph{
		Nodes: make(map[string]*Node),
	}
}

// AddNode adds a new node if it doesn't exist.
func (g *Graph) AddNode(name string) {
	if _, exists := g.Nodes[name]; !exists {
		g.Nodes[name] = &Node{Name: name, Edges: []string{}}
	}
}

// AddEdge adds a directed edge from one node to another.
func (g *Graph) AddEdge(from, to string) {
	g.AddNode(from)
	g.AddNode(to)
	g.Nodes[from].Edges = append(g.Nodes[from].Edges, to)
}

// SaveToFile saves the graph to a JSON file.
func (g *Graph) SaveToFile(filename string) error {
	file, err := os.Create(filename)
	if err != nil {
		return err
	}
	defer file.Close()
	encoder := json.NewEncoder(file)
	encoder.SetIndent("", "  ")
	return encoder.Encode(g)
}

// LoadFromFile loads a graph from a JSON file.
func LoadFromFile(filename string) (*Graph, error) {
	file, err := os.Open(filename)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	var g Graph
	err = json.NewDecoder(file).Decode(&g)
	if g.Nodes == nil {
		g.Nodes = make(map[string]*Node)
	}
	return &g, err
}
