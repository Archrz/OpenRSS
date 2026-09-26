package main

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"os"
	"path/filepath"

	"openrss/feed"
	"openrss/store"
)

func addFeedHandler(s *store.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			URL string `json:"url"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.URL == "" {
			jsonError(w, http.StatusBadRequest, errors.New("missing or invalid url"))
			return
		}
		id, err := s.AddFeed(body.URL, body.URL, "")
		if err != nil {
			jsonError(w, http.StatusInternalServerError, err)
			return
		}
		added, err := feed.RefreshFeed(s, store.Feed{ID: id, URL: body.URL, Title: body.URL})
		resp := map[string]any{"id": id, "added": added}
		if err != nil {
			resp["error"] = err.Error()
		}
		writeJSON(w, resp)
	}
}

func feedByIDHandler(s *store.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, err := pathID(r)
		if err != nil {
			http.NotFound(w, r)
			return
		}
		if err := s.RemoveFeed(id); err != nil {
			jsonError(w, http.StatusInternalServerError, err)
			return
		}
		writeJSON(w, map[string]bool{"ok": true})
	}
}

func feedRefreshHandler(s *store.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, err := pathID(r)
		if err != nil {
			http.NotFound(w, r)
			return
		}
		f, err := s.GetFeed(id)
		if err != nil {
			http.NotFound(w, r)
			return
		}
		added, err := feed.RefreshFeed(s, f)
		resp := map[string]any{"added": added}
		if err != nil {
			resp["error"] = err.Error()
		}
		writeJSON(w, resp)
	}
}

func refreshAllHandler(s *store.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, map[string]any{"results": feed.RefreshAll(s)})
	}
}

func markAllReadHandler(s *store.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			FeedID int64 `json:"feedId"`
		}
		json.NewDecoder(r.Body).Decode(&body)
		if err := s.MarkAllRead(body.FeedID); err != nil {
			jsonError(w, http.StatusInternalServerError, err)
			return
		}
		writeJSON(w, map[string]bool{"ok": true})
	}
}

func statsHandler(s *store.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		feedCount, articleCount, err := s.Stats()
		if err != nil {
			jsonError(w, http.StatusInternalServerError, err)
			return
		}
		writeJSON(w, map[string]int{"feeds": feedCount, "articles": articleCount})
	}
}

func settingsHandler(s *store.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			articleCap, err := s.GetArticleCap()
			if err != nil {
				jsonError(w, http.StatusInternalServerError, err)
				return
			}
			writeJSON(w, map[string]int{"articleCap": articleCap})

		case http.MethodPost:
			var body struct {
				ArticleCap int `json:"articleCap"`
			}
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.ArticleCap < 0 {
				jsonError(w, http.StatusBadRequest, errors.New("invalid articleCap"))
				return
			}
			if err := s.SetArticleCap(body.ArticleCap); err != nil {
				jsonError(w, http.StatusInternalServerError, err)
				return
			}
			writeJSON(w, map[string]bool{"ok": true})
		}
	}
}

func clearArticlesHandler(s *store.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if err := s.ClearArticles(); err != nil {
			jsonError(w, http.StatusInternalServerError, err)
			return
		}
		writeJSON(w, map[string]bool{"ok": true})
	}
}

func opmlImportHandler(s *store.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		file, _, err := r.FormFile("opml")
		if err != nil {
			jsonError(w, http.StatusBadRequest, err)
			return
		}
		defer file.Close()
		data, err := io.ReadAll(file)
		if err != nil {
			jsonError(w, http.StatusBadRequest, err)
			return
		}
		imported, err := ImportOPMLData(s, data)
		if err != nil {
			jsonError(w, http.StatusBadRequest, err)
			return
		}
		writeJSON(w, map[string]int{"imported": imported})
	}
}

func opmlExportHandler(s *store.Store, dbPath string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		feeds, err := s.ListFeeds()
		if err != nil {
			jsonError(w, http.StatusInternalServerError, err)
			return
		}
		path := filepath.Join(filepath.Dir(dbPath), "openrss-export.opml")
		if err := os.WriteFile(path, []byte(ExportOPML(feeds)), 0o644); err != nil {
			jsonError(w, http.StatusInternalServerError, err)
			return
		}
		writeJSON(w, map[string]string{"path": path})
	}
}
