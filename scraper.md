# 🕸️ Scraper Module Guide (`scraper/fetch.go`)

This file is handled by **Abin** and is responsible for collecting data from sources like Wikipedia and building the knowledge graph.

---

##  Responsibilities

- Fetch structured data (e.g., titles, summaries, links) from Wikipedia or other public sources.
- Parse the data into entities and relationships.
- Build the knowledge graph using `shared/graph.go`.
- Save the graph into `data/graph.json`.

---

##  Input

- Hardcoded or dynamically chosen topic titles.
- Optionally: category/tag to fetch multiple articles.

---

##  Output

- A `graph.json` file inside the `data/` folder with entity relationships.
- Format: JSON as per `shared.Graph`.

---

##  Example

```go
graph := shared.NewGraph()
graph.AddEdge("Go", "Programming Language")
graph.AddEdge("Go", "Concurrency")
graph.SaveToFile("data/graph.json")
```

Output: `data/graph.json`

```json
{
  "nodes": {
    "Go": { "name": "Go", "edges": ["Programming Language", "Concurrency"] },
    "Programming Language": { "name": "Programming Language", "edges": [] },
    "Concurrency": { "name": "Concurrency", "edges": [] }
  }
}
```

---

##  Shared Code Used

| Function            | From `shared/graph.go` | Description                   |
|---------------------|------------------------|-------------------------------|
| `NewGraph()`        | ✅                     | Creates new graph             |
| `AddNode()`         | ✅                     | Adds entity                   |
| `AddEdge()`         | ✅                     | Adds relation                 |
| `SaveToFile()`      | ✅                     | Saves graph to JSON           |

---

##  Branch & Git Workflow

```bash
# Create branch
git checkout -b feature/scraper

# After coding
git add .
git commit -m "Implement basic Wikipedia scraper"
git push origin feature/scraper
```

When ready to merge:

```bash
git checkout main
git pull origin main
git merge feature/scraper
git push origin main
```

If working parallel with API:

```bash
git pull --rebase origin main
```

---

##  Will API use scraper code?

 No. The API just reads the file `data/graph.json` — it does **not import or call** the scraper logic.