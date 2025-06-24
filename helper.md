

#  `shared/graph.go` — Full Breakdown

This file defines the **shared data structure** used by **both the scraper and the API**.

## 🔹 Structs

### 1. `Node`

```go
type Node struct {
	Name  string   `json:"name"`
	Edges []string `json:"edges"`
}
```

* Represents one entity in the knowledge graph (like “Albert Einstein”).
* `Edges` holds a list of other node names it’s connected to (relationships).

---

### 2. `Graph`

```go
type Graph struct {
	Nodes map[string]*Node `json:"nodes"`
}
```

* A map-based graph: keys are node names, values are `*Node`.
* Simple, scalable, and fast for lookups.

---

## 🔹 Functions

###  `NewGraph()`

Creates and returns an empty graph.

```go
func NewGraph() *Graph {
	return &Graph{Nodes: make(map[string]*Node)}
}
```

Used by both scraper and API to initialize the graph.

---

###  `AddNode(name string)`

Adds a node to the graph if it doesn’t already exist.

```go
func (g *Graph) AddNode(name string) {
	if _, exists := g.Nodes[name]; !exists {
		g.Nodes[name] = &Node{Name: name, Edges: []string{}}
	}
}
```

---

###  `AddEdge(from, to string)`

Adds a connection (edge) from one node to another.

```go
func (g *Graph) AddEdge(from, to string) {
	g.AddNode(from)
	g.AddNode(to)
	g.Nodes[from].Edges = append(g.Nodes[from].Edges, to)
}
```

---

###  `SaveToFile(filename string)`

Stores the graph in a JSON file.

```go
func (g *Graph) SaveToFile(filename string) error
```

Used by the scraper to save data for the API to read.

---

###  `LoadFromFile(filename string)`

Reads a saved graph from a JSON file.

```go
func LoadFromFile(filename string) (*Graph, error)
```

Used by the API to load graph on startup.

---

##  Who Uses What?

| Function         | Used By      | Purpose                       |
| ---------------- | ------------ | ----------------------------- |
| `NewGraph()`     | Scraper, API | Start with an empty graph     |
| `AddNode()`      | Scraper      | Create a new entity           |
| `AddEdge()`      | Scraper      | Create relationships          |
| `SaveToFile()`   | Scraper      | Save the graph after scraping |
| `LoadFromFile()` | API          | Load graph to answer queries  |

---

#  Roles of Each File

###  Scraper (`scraper/fetch.go`)

* Uses Wikipedia API
* Creates graph
* Adds entities and relationships
* Saves to `data/graph.json` using `SaveToFile`

###  API Server (`api/server.go`)

* Loads graph using `LoadFromFile`
* Starts web server with `/query?q=...`
* Returns matching node and its edges

---

# 🎯 First Increment: Full Working Demo

### 🧪 Example Input

**Scraper writes:**

```go
graph.AddEdge("Albert Einstein", "Physics")
graph.AddEdge("Albert Einstein", "Nobel Prize")
graph.SaveToFile("data/graph.json")
```

Graph JSON will look like:

```json
{
  "nodes": {
    "Albert Einstein": {
      "name": "Albert Einstein",
      "edges": ["Physics", "Nobel Prize"]
    },
    "Physics": {
      "name": "Physics",
      "edges": []
    },
    "Nobel Prize": {
      "name": "Nobel Prize",
      "edges": []
    }
  }
}
```

---

### 📥 API Input

GET Request:

```
http://localhost:8080/query?q=Albert Einstein
```

---

### 📤 Output JSON

```json
{
  "name": "Albert Einstein",
  "edges": ["Physics", "Nobel Prize"]
}
```

---

#  Testing Instructions

## Scraper

```bash
go run scraper/fetch.go
```

* Check if `data/graph.json` is created
* Open the file and validate the structure

## API

```bash
go run api/server.go
```

* Open browser or use curl:

```bash
curl "http://localhost:8080/query?q=Albert Einstein"
```

---

# 🔀 Collaboration & Merge Plan

###  Each Member Should:

| Task                  | Member A          | Member B          |
| --------------------- | ----------------- | ----------------- |
| Create branch         | `feature/scraper` | `feature/query`   |
| Work in               | `scraper/`        | `api/`            |
| Use `shared/graph.go` | Yes               | Yes               |
| Push code             | To own branch     | To own branch     |
| Pull main             | Before PR merge   | Before PR merge   |
| PR review             | Create and assign | Create and assign |

---

###  Merge to `main`

```bash
# Pull latest main before merge
git checkout main
git pull origin main

# Merge your branch
git merge feature/scraper  # or feature/query

# Push merged main
git push origin main
```

---

#  Coordination Tips

1. **Avoid editing `shared/` without telling the other member**
2. Use GitHub issues or a shared doc for task division
3. Update each other before merging to `main`
4. Commit frequently but meaningfully
5. If there's a conflict, resolve by:

   ```bash
   git pull --rebase origin main
   ```

---
