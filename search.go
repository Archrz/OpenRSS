package main

import (
	"regexp"
	"sort"
	"strings"
	"time"
)

type SearchResult struct {
	Article         Article  `json:"article"`
	Matched         int      `json:"matched"`
	Total           int      `json:"total"`
	Ratio           float64  `json:"ratio"`
	MatchedKeywords []string `json:"matchedKeywords"`
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
func SearchArticles(articles []Article, query string) []SearchResult {
	keywords := Keywords(query)
	if len(keywords) == 0 {
		return nil
	}
	results := []SearchResult{}
	for _, a := range articles {
		haystack := normalizeForSearch(a.Title + " " + a.Summary + " " + a.Content)
		matchedKeywords := []string{}
		for _, kw := range keywords {
			if strings.Contains(haystack, kw) {
				matchedKeywords = append(matchedKeywords, kw)
			}
		}
		if len(matchedKeywords) == 0 {
			continue
		}
		a.Content = ""
		results = append(results, SearchResult{
			Article:         a,
			Matched:         len(matchedKeywords),
			Total:           len(keywords),
			Ratio:           float64(len(matchedKeywords)) / float64(len(keywords)),
			MatchedKeywords: matchedKeywords,
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

// [from, to), zero means unbounded
func FilterByDateRange(articles []Article, from, to time.Time) []Article {
	if from.IsZero() && to.IsZero() {
		return articles
	}
	out := make([]Article, 0, len(articles))
	for _, a := range articles {
		if !from.IsZero() && a.PubDate.Before(from) {
			continue
		}
		if !to.IsZero() && !a.PubDate.Before(to) {
			continue
		}
		out = append(out, a)
	}
	return out
}
