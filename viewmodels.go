package main

import (
	"fmt"
	"html/template"

	"openrss/store"
)

type feedRow struct {
	store.Feed
	Unread int
	Active bool
}

type articleRow struct {
	store.Article
	Selected bool
}

type readerView struct {
	store.Article
	ContentHTML template.HTML
}

type searchView struct {
	Query         string
	Keywords      []string
	Results       []SearchResult
	TotalArticles int
}

func buildSearchView(query string, results []SearchResult, totalArticles int) searchView {
	return searchView{
		Query:         query,
		Keywords:      Keywords(query),
		Results:       results,
		TotalArticles: totalArticles,
	}
}

func feedRows(feeds []store.Feed, counts map[int64]int, selected int64) []feedRow {
	total := 0
	for _, c := range counts {
		total += c
	}
	rows := make([]feedRow, 0, len(feeds)+1)
	rows = append(rows, feedRow{Feed: store.Feed{Title: "All articles"}, Unread: total, Active: selected == 0})
	for _, f := range feeds {
		rows = append(rows, feedRow{Feed: f, Unread: counts[f.ID], Active: f.ID == selected})
	}
	return rows
}

func articleRows(articles []store.Article, selected int64) []articleRow {
	rows := make([]articleRow, len(articles))
	for i, a := range articles {
		rows[i] = articleRow{Article: a, Selected: a.ID == selected}
	}
	return rows
}

func unreadCounts(articles []store.Article) map[int64]int {
	counts := map[int64]int{}
	for _, a := range articles {
		if !a.Read {
			counts[a.FeedID]++
		}
	}
	return counts
}

func feedTitleLabel(feeds []store.Feed, selected int64) string {
	if selected == 0 {
		return "All articles"
	}
	for _, f := range feeds {
		if f.ID == selected {
			return f.Title
		}
	}
	return "All articles"
}

func itemCountLabel(n int) string {
	if n == 1 {
		return "1 item"
	}
	return fmt.Sprintf("%d items", n)
}
