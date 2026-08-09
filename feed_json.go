package main

import (
	"encoding/json"
	"net/url"
	"time"
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
		Name   string `json:"name"`
		Avatar string `json:"avatar"`
	} `json:"authors"`
	Author struct {
		Name   string `json:"name"`
		Avatar string `json:"avatar"`
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

func jsonAuthor(it jsonFeedItem) (name, avatar string) {
	name, avatar = it.Author.Name, it.Author.Avatar
	if len(it.Authors) > 0 && it.Authors[0].Name != "" {
		name, avatar = it.Authors[0].Name, it.Authors[0].Avatar
	}
	return name, avatar
}

func jsonPublished(it jsonFeedItem) string {
	if it.DatePublished != "" {
		return it.DatePublished
	}
	return it.DateModified
}

func parseJSONFeed(data []byte, fetchedAt time.Time) (title string, articles []Article, err error) {
	var f jsonFeed
	if err := json.Unmarshal(data, &f); err != nil {
		return "", nil, err
	}
	for _, it := range f.Items {
		link := jsonLink(it)
		author, avatar := jsonAuthor(it)
		if base, err := url.Parse(link); err == nil && base.IsAbs() {
			avatar = resolveURL(avatar, base)
		}
		content := sanitizer.Sanitize(resolveRelativeURLs(jsonContent(it), link))
		articles = append(articles, Article{
			Link:         link,
			Title:        it.Title,
			Author:       author,
			AuthorAvatar: avatar,
			PubDate:      parseDate(jsonPublished(it)),
			Summary:      summarize(content, 220),
			Content:      content,
			FetchedAt:    fetchedAt,
		})
	}
	return f.Title, articles, nil
}
