package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
)

type Word struct {
	Word string `json:"word"`
}

func getSynonyms(word string, max int) ([]string, error) {
	apiURL := fmt.Sprintf("https://api.datamuse.com/words?rel_syn=%s&max=%d", url.QueryEscape(word), max)
	resp, err := http.Get(apiURL)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var results []Word
	if err := json.NewDecoder(resp.Body).Decode(&results); err != nil {
		return nil, err
	}

	synonyms := make([]string, 0, len(results))
	for _, w := range results {
		synonyms = append(synonyms, w.Word)
	}
	return synonyms, nil
}

func main() {
	var word string
	// var count int
	fmt.Println("Word:")
	fmt.Scan(&word)
	// fmt.Println("count:")
	synonyms, err := getSynonyms(word, 10)
	if err != nil {
		fmt.Println("Error:", err)
		return
	}

	fmt.Printf("Top synonyms for '%s':\n", word)
	for _, s := range synonyms {
		fmt.Println("-", s)
	}
}
