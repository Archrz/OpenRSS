package main

import (
	"embed"
	"encoding/json"
	"errors"
	"io"
	"io/fs"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

//go:embed frontend
var frontendFS embed.FS

func staticHandler() http.Handler {
	sub, err := fs.Sub(frontendFS, "frontend")
	if err != nil {
		log.Fatalf("embed frontend dir: %v", err)
	}
	return http.FileServerFS(sub)
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(v); err != nil {
		log.Printf("write json: %v", err)
	}
}

func jsonError(w http.ResponseWriter, status int, err error) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
}

func pathID(r *http.Request) (int64, error) {
	return strconv.ParseInt(r.PathValue("id"), 10, 64)
}

func feedsHandler(s *Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			feeds, err := s.ListFeeds()
			if err != nil {
				jsonError(w, http.StatusInternalServerError, err)
				return
			}
			writeJSON(w, feeds)

		case http.MethodPost:
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
			added, err := RefreshFeed(s, Feed{ID: id, URL: body.URL, Title: body.URL})
			resp := map[string]any{"id": id, "added": added}
			if err != nil {
				resp["error"] = err.Error()
			}
			writeJSON(w, resp)
		}
	}
}

func feedByIDHandler(s *Store) http.HandlerFunc {
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

func feedRefreshHandler(s *Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, err := pathID(r)
		if err != nil {
			http.NotFound(w, r)
			return
		}
		feeds, err := s.ListFeeds()
		if err != nil {
			jsonError(w, http.StatusInternalServerError, err)
			return
		}
		var feed *Feed
		for i := range feeds {
			if feeds[i].ID == id {
				feed = &feeds[i]
				break
			}
		}
		if feed == nil {
			http.NotFound(w, r)
			return
		}
		added, err := RefreshFeed(s, *feed)
		resp := map[string]any{"added": added}
		if err != nil {
			resp["error"] = err.Error()
		}
		writeJSON(w, resp)
	}
}

func refreshAllHandler(s *Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, map[string]any{"results": RefreshAll(s)})
	}
}

func articlesHandler(s *Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var feedID int64
		if v := r.URL.Query().Get("feed"); v != "" {
			feedID, _ = strconv.ParseInt(v, 10, 64)
		}
		articles, err := s.ListArticles(feedID)
		if err != nil {
			jsonError(w, http.StatusInternalServerError, err)
			return
		}
		writeJSON(w, articles)
	}
}

// raw HTML length, not word count
const minFullContentLength = 1500

func articleByIDHandler(s *Store) http.HandlerFunc {
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
			// already substantial, skip fetch
			if err := s.MarkArticleFull(a.ID); err != nil {
				log.Printf("mark article %d full: %v", a.ID, err)
			} else {
				a.Full = true
			}
		} else if !a.Full && a.Link != "" {
			if content, err := FetchFullArticle(a.Link); err != nil {
				log.Printf("fetch full article %s: %v", a.Link, err)
			} else {
				content = cacheArticleImages(content)
				if err := s.SetArticleContent(a.ID, content); err != nil {
					log.Printf("save full article %s: %v", a.Link, err)
				} else {
					a.Content = content
					a.Full = true
				}
			}
		}
		if strings.HasPrefix(a.AuthorAvatar, "http") {
			if local, ok := cacheImage(a.AuthorAvatar); ok {
				if err := s.SetArticleAuthorAvatar(a.ID, local); err != nil {
					log.Printf("save author avatar for article %d: %v", a.ID, err)
				} else {
					a.AuthorAvatar = local
				}
			}
		}
		writeJSON(w, a)
	}
}

func markAllReadHandler(s *Store) http.HandlerFunc {
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

func searchHandler(s *Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query().Get("q")
		articles, err := s.ListArticlesWithContent()
		if err != nil {
			jsonError(w, http.StatusInternalServerError, err)
			return
		}
		from, _ := time.Parse("2006-01-02", r.URL.Query().Get("from"))
		to, err := time.Parse("2006-01-02", r.URL.Query().Get("to"))
		if err == nil {
			to = to.Add(24 * time.Hour) // make the end date inclusive of that whole day
		}
		articles = FilterByDateRange(articles, from, to)
		writeJSON(w, SearchArticles(articles, q))
	}
}

func statsHandler(s *Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		feedCount, articleCount, err := s.Stats()
		if err != nil {
			jsonError(w, http.StatusInternalServerError, err)
			return
		}
		writeJSON(w, map[string]int{"feeds": feedCount, "articles": articleCount})
	}
}

func clearArticlesHandler(s *Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if err := s.ClearArticles(); err != nil {
			jsonError(w, http.StatusInternalServerError, err)
			return
		}
		if imageCacheDir != "" {
			if err := os.RemoveAll(imageCacheDir); err != nil {
				log.Printf("clear image cache: %v", err)
			} else {
				os.MkdirAll(imageCacheDir, 0o755)
			}
		}
		writeJSON(w, map[string]bool{"ok": true})
	}
}

func opmlImportHandler(s *Store) http.HandlerFunc {
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

// next to db, simpler than a save dialog
func opmlExportHandler(s *Store, dbPath string) http.HandlerFunc {
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
