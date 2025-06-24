#  API Server Module Guide (`api/server.go`)

This file is handled by **Fellah** and is responsible for loading the knowledge graph and answering user queries over HTTP.

---

##  Responsibilities

- Load the graph from `data/graph.json`.
- Start an HTTP server.
- Accept queries like `/query?q=Albert Einstein`.
- Return JSON response showing the entity and its edges.

---

##  Input

- GET request from browser or client like curl:
```
/query?q=<entity-name>
```

---

##  Output

- JSON response like:

```json
{
  "name": "Albert Einstein",
  "edges": ["Physics", "Nobel Prize"]
}
```

---

##  Example

Browser/curl request:

```
http://localhost:8080/query?q=Albert Einstein
```

Server response:

```json
{
  "name": "Albert Einstein",
  "edges": ["Physics", "Nobel Prize"]
}
```

---

##  Shared Code Used

| Function         | From `shared/graph.go` | Description             |
|------------------|------------------------|-------------------------|
| `LoadFromFile()` | ✅                     | Load graph from JSON    |
| `Graph.Nodes`    | ✅                     | Search for queried node |

---

##  Branch & Git Workflow

```bash
# Create branch
git checkout -b feature/query

# After coding
git add .
git commit -m "Add query API for graph"
git push origin feature/query
```

When ready to merge:

```bash
git checkout main
git pull origin main
git merge feature/query
git push origin main
```

If working parallel with scraper:

```bash
git pull --rebase origin main
```

---

##  Will scraper use API code?

 No. Scraper only saves data. The API is read-only and self-contained.