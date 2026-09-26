package main

import (
	"regexp"
	"sort"
	"strings"

	"openrss/store"
)

type SearchResult struct {
	Article store.Article `json:"article"`
	Matched int     `json:"matched"`
	Total   int     `json:"total"`
	Ratio   float64 `json:"ratio"`
}

var normalizeRE = regexp.MustCompile(`[^a-z0-9\s-]+|\s+`)

// "zero day" searches for zero day as a phrase or word.
func normalizeForSearch(s string) string {
	return strings.TrimSpace(normalizeRE.ReplaceAllString(strings.ToLower(s), " "))
}

var keywordRE = regexp.MustCompile(`"([^"]+)"|([^\s,]+)`)

// "quoted phrase" is one keyword, e.g. "zero day"
func Keywords(query string) []string {
	var kws []string
	for _, m := range keywordRE.FindAllStringSubmatch(query, -1) {
		word := m[1]
		if word == "" {
			word = m[2]
		}
		word = normalizeForSearch(word)
		if len(word) > 1 {
			kws = append(kws, word)
		}
	}
	return kws
}

// ranks by match count, then ratio
func SearchArticles(articles []store.Article, query string) []SearchResult {
	keywords := Keywords(query)
	if len(keywords) == 0 {
		return nil
	}
	results := []SearchResult{}
	for _, a := range articles {
		haystack := normalizeForSearch(a.Title + " " + a.Summary + " " + a.Content)
		matched := 0
		for _, kw := range keywords {
			if strings.Contains(haystack, kw) {
				matched++
			}
		}
		if matched == 0 {
			continue
		}
		a.Content = ""
		results = append(results, SearchResult{
			Article: a,
			Matched: matched,
			Total:   len(keywords),
			Ratio:   float64(matched) / float64(len(keywords)),
		})
	}
	sort.SliceStable(results, func(i, j int) bool {
		if results[i].Matched != results[j].Matched {
			return results[i].Matched > results[j].Matched
		}
		return results[i].Ratio > results[j].Ratio
	})
	return results
}
