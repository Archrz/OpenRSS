package main

import (
	"fmt"
	"html/template"
	"log"
	"net/http"
	"time"

	"openrss/feed"
	"openrss/store"
)

// GET /{$} — the full app shell, initial data server-rendered
func shellHandler(s *store.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		feeds, err := s.ListFeeds()
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		articles, err := s.ListArticles(0)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		_, articleCount, err := s.Stats()
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		data := map[string]any{
			"SelectedFeedID": int64(0),
			"Feeds":          feedRows(feeds, unreadCounts(articles), 0),
			"Articles":       articleRows(articles, 0),
			"FeedTitleLabel": feedTitleLabel(feeds, 0),
			"ItemCountLabel": itemCountLabel(len(articles)),
			"SearchInitial":  buildSearchView("", nil, articleCount),
		}
		html, err := render("shell", data)
		writeHTML(w, html, err)
	}
}

// GET /fragments/feeds?selected=<id>
func feedListFragmentHandler(s *store.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		feeds, err := s.ListFeeds()
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		articles, err := s.ListArticles(0)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		selected := queryID(r, "selected")
		html, err := render("feedList", feedRows(feeds, unreadCounts(articles), selected))
		writeHTML(w, html, err)
	}
}

// GET /fragments/articles?feed=<id>&selected=<id>
func articleListFragmentHandler(s *store.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		feedID := queryID(r, "feed")
		selected := queryID(r, "selected")
		feeds, err := s.ListFeeds()
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		articles, err := s.ListArticles(feedID)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.Header().Set("X-Feed-Title", feedTitleLabel(feeds, feedID))
		w.Header().Set("X-Item-Count", itemCountLabel(len(articles)))
		html, err := render("articleList", articleRows(articles, selected))
		writeHTML(w, html, err)
	}
}

const minFullContentLength = 1500

// GET /fragments/article/{id} — also marks read, fetches full text on demand
func articleFragmentHandler(s *store.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, err := pathID(r)
		if err != nil {
			http.NotFound(w, r)
			return
		}
		a, err := s.GetArticle(id)
		if err != nil {
			http.NotFound(w, r)
			return
		}
		if !a.Full && len(a.Content) >= minFullContentLength {
			if err := s.MarkArticleFull(a.ID); err != nil {
				log.Printf("mark article %d full: %v", a.ID, err)
			} else {
				a.Full = true
			}
		} else if !a.Full && a.Link != "" {
			if content, err := feed.FetchFullArticle(a.Link); err != nil {
				log.Printf("fetch full article %s: %v", a.Link, err)
			} else if err := s.SetArticleContent(a.ID, content); err != nil {
				log.Printf("save full article %s: %v", a.Link, err)
			} else {
				a.Content = content
				a.Full = true
			}
		}
		w.Header().Set("X-Feed-Title", a.FeedTitle)
		w.Header().Set("X-Article-Link", a.Link)
		html, err := render("readerContent", readerView{Article: a, ContentHTML: template.HTML(a.Content)})
		writeHTML(w, html, err)
	}
}

// GET /fragments/search?q=&from=&to=
func searchFragmentHandler(s *store.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query().Get("q")
		keywords := Keywords(q)

		_, articleCount, err := s.Stats()
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		var results []SearchResult
		if len(keywords) > 0 {
			from, _ := time.Parse("2006-01-02", r.URL.Query().Get("from"))
			to, toErr := time.Parse("2006-01-02", r.URL.Query().Get("to"))
			if toErr == nil {
				to = to.Add(24 * time.Hour)
			}
			candidates, err := s.SearchCandidates(keywords, from, to)
			if err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			results = SearchArticles(candidates, q)
		}

		w.Header().Set("X-Result-Count", fmt.Sprintf("%d", len(results)))
		html, err := render("searchDynamic", buildSearchView(q, results, articleCount))
		writeHTML(w, html, err)
	}
}
