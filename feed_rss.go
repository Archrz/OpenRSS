package main

import (
	"encoding/xml"
	"regexp"
	"strings"
	"time"
)

type rssItem struct {
	Title       string `xml:"title"`
	Link        string `xml:"link"`
	GUID        string `xml:"guid"`
	PubDate     string `xml:"pubDate"`
	Creator     string `xml:"creator"` // dc:creator, namespace-agnostic match
	Author      string `xml:"author"`
	Description string `xml:"description"`
	Encoded     string `xml:"encoded"` // content:encoded, namespace-agnostic match
}

type rssXML struct {
	Channel struct {
		Title string    `xml:"title"`
		Items []rssItem `xml:"item"`
	} `xml:"channel"`
}

// RSS author format "email (Full Name)"
var rssAuthorRE = regexp.MustCompile(`^([^\s()]+@[^\s()]+)\s*(?:\(([^)]*)\))?$`)

func parseRSSAuthor(raw string) (name, email string) {
	raw = strings.TrimSpace(raw)
	if m := rssAuthorRE.FindStringSubmatch(raw); m != nil {
		email = m[1]
		name = strings.TrimSpace(m[2])
		if name == "" {
			name = email
		}
		return name, email
	}
	return raw, ""
}

func rssContent(it rssItem) string {
	if it.Encoded != "" {
		return it.Encoded
	}
	return it.Description
}

func rssLink(it rssItem) string {
	if it.Link != "" {
		return it.Link
	}
	return it.GUID
}

// prefers dc:creator's plain name over the <author> email form, if present
func rssAuthor(it rssItem) (name, email string) {
	if it.Author != "" {
		name, email = parseRSSAuthor(it.Author)
	}
	if it.Creator != "" {
		name = it.Creator
	}
	return name, email
}

func parseRSSFeed(data []byte, now time.Time) (title string, articles []Article, err error) {
	var f rssXML
	if err := xml.Unmarshal(data, &f); err != nil {
		return "", nil, err
	}
	for _, it := range f.Channel.Items {
		link := rssLink(it)
		author, email := rssAuthor(it)
		content := sanitizer.Sanitize(resolveRelativeURLs(rssContent(it), link))
		articles = append(articles, Article{
			Link:        link,
			Title:       it.Title,
			Author:      author,
			AuthorEmail: email,
			PubDate:     parseDate(it.PubDate),
			Summary:     summarize(content, 220),
			Content:     content,
			FetchedAt:   now,
		})
	}
	return f.Channel.Title, articles, nil
}
