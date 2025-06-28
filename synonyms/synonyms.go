package synonyms

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
)

type Word struct {
	Word string `json:"word"`
}

func GetSynonyms(word string, max int) ([]string, error) {
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
