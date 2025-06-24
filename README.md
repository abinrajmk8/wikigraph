#  WikiGraph: Knowledge Graph Search Engine (Go)

WikiGraph is a lightweight, Go-powered knowledge graph search engine. It fetches information from Wikipedia, builds a graph of entities and their relationships, and provides a simple API for answering user queries.

---

##  Features

- 🌐 Scrapes Wikipedia to extract meaningful relationships
- 📊 Stores data in a JSON-based graph (nodes + edges)
- 🔎 Supports query-based search (e.g., "Albert Einstein")
- ⚡ Fast and minimal API built with Go's `net/http`
- 🔄 Modular design with shared graph logic
- 🧩 Easily extendable for new data sources and NLP

---

## 🛠️ Tech Stack

| Layer     | Technology        |
|-----------|-------------------|
| Language  | Go (Golang)       |
| Web API   | net/http          |
| Scraping  | Wikipedia API     |
| Storage   | JSON-based graph  |
| Helpers   | Custom Go modules |

---

##  Folder Structure

```
wikigraph/
│
├── api/             # HTTP API server
│   └── server.go
│
├── scraper/         # Wikipedia scraper
│   └── fetch.go
│
├── shared/          # Graph module (shared)
│   └── graph.go
│
├── data/            # Graph storage (graph.json)
│   └── .gitkeep
│
├── README.md
├── .gitignore
└── go.mod
```

---

##  How It Works

### 1. Scraper

Fetches Wikipedia topics and builds a graph:

```go
graph.AddEdge("Albert Einstein", "Physics")
graph.AddEdge("Albert Einstein", "Nobel Prize")
graph.SaveToFile("data/graph.json")
```

Creates a JSON file:

```json
{
  "nodes": {
    "Albert Einstein": {
      "name": "Albert Einstein",
      "edges": ["Physics", "Nobel Prize"]
    }
  }
}
```

---

### 2. API Server

Loads the graph and starts an HTTP server:

```bash
http://localhost:8080/query?q=Albert Einstein
```

Responds with:

```json
{
  "name": "Albert Einstein",
  "edges": ["Physics", "Nobel Prize"]
}
```

---

##  Installation & Usage

### 1. Clone the Repository

```bash
git clone https://github.com/your-username/wikigraph.git
cd wikigraph
```

### 2. Run the Scraper

```bash
go run scraper/fetch.go
```

Generates `data/graph.json`.

### 3. Run the API Server

```bash
go run api/server.go
```

Now open in browser or run:

```bash
curl "http://localhost:8080/query?q=Albert Einstein"
```

---

##  Sample Input & Output

### Input

Wikipedia nodes added:

```go
graph.AddEdge("Go", "Concurrency")
graph.AddEdge("Go", "Programming Language")
```

### Output

```json
{
  "name": "Go",
  "edges": ["Concurrency", "Programming Language"]
}
```

---

##  Advantages

- Easy to understand and extend
- Built entirely in Go — no external services required
- Real-time query handling
- Clean and modular codebase
- Portable and works offline after data is scraped

---

##  Team Members

- **Abin** – Scraper developer
- **Fellah** – API developer

---

## 📌 Future Enhancements

- Use real NLP to parse natural language queries
- Crawl additional data sources
- Add graph visualization to the frontend
- Connect to Neo4j or Dgraph

---

## 📄 License

This project is for educational use. All content scraped from Wikipedia follows their [Terms of Use](https://foundation.wikimedia.org/wiki/Terms_of_Use).

---

## ❤️ Acknowledgements

- Wikipedia for data
- Go community