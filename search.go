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

var (
	nonWordRE    = regexp.MustCompile(`[^a-z0-9\s-]`)
	multiSpaceRE = regexp.MustCompile(`\s+`)
)

// "zero day" searches for zero day as a idea or concept
func normalizeForSearch(s string) string {
	s = nonWordRE.ReplaceAllString(strings.ToLower(s), " ")
	return strings.TrimSpace(multiSpaceRE.ReplaceAllString(s, " "))
}

func isSeparator(r rune) bool {
	return r == ' ' || r == ',' || r == '\t' || r == '\n'
}

// "quoted phrase" is one keyword, e.g. "zero day"
func Keywords(query string) []string {
	kws := []string{}
	runes := []rune(query)
	n := len(runes)
	i := 0
	for i < n {
		for i < n && isSeparator(runes[i]) {
			i++
		}
		if i >= n {
			break
		}
		var word string
		if runes[i] == '"' {
			i++
			start := i
			for i < n && runes[i] != '"' {
				i++
			}
			word = strings.TrimSpace(string(runes[start:i]))
			if i < n {
				i++ // skip closing quote
			}
		} else {
			start := i
			for i < n && !isSeparator(runes[i]) {
				i++
			}
			word = string(runes[start:i])
		}
		word = strings.ToLower(word)
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
		a.Content = "" // keep search responses light; reader fetches content separately
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
