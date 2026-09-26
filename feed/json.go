package feed

import (
	"encoding/json"
	"time"

	"openrss/store"
)

// JSON Feed 1.1 (jsonfeed.org).
type jsonFeedItem struct {
	ID            string `json:"id"`
	URL           string `json:"url"`
	Title         string `json:"title"`
	ContentHTML   string `json:"content_html"`
	ContentText   string `json:"content_text"`
	Summary       string `json:"summary"`
	DatePublished string `json:"date_published"`
	DateModified  string `json:"date_modified"`
	Authors       []struct {
		Name string `json:"name"`
	} `json:"authors"`
	Author struct {
		Name string `json:"name"`
	} `json:"author"`
}

type jsonFeed struct {
	Title string         `json:"title"`
	Items []jsonFeedItem `json:"items"`
}

func jsonContent(it jsonFeedItem) string {
	if it.ContentHTML != "" {
		return it.ContentHTML
	}
	if it.ContentText != "" {
		return it.ContentText
	}
	return it.Summary
}

func jsonLink(it jsonFeedItem) string {
	if it.URL != "" {
		return it.URL
	}
	return it.ID
}

func jsonAuthor(it jsonFeedItem) string {
	if len(it.Authors) > 0 && it.Authors[0].Name != "" {
		return it.Authors[0].Name
	}
	return it.Author.Name
}

func jsonPublished(it jsonFeedItem) string {
	if it.DatePublished != "" {
		return it.DatePublished
	}
	return it.DateModified
}

func parseJSONFeed(data []byte, fetchedAt time.Time) (title string, articles []store.Article, err error) {
	var f jsonFeed
	if err := json.Unmarshal(data, &f); err != nil {
		return "", nil, err
	}
	for _, it := range f.Items {
		link := jsonLink(it)
		articles = append(articles, buildArticle(link, it.Title, jsonAuthor(it), "", jsonPublished(it), jsonContent(it), fetchedAt))
	}
	return f.Title, articles, nil
}
