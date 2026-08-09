package main

import (
	"encoding/xml"
	"time"
)

type atomEntry struct {
	Title string `xml:"title"`
	Links []struct {
		Href string `xml:"href,attr"`
		Rel  string `xml:"rel,attr"`
	} `xml:"link"`
	ID        string `xml:"id"`
	Published string `xml:"published"`
	Updated   string `xml:"updated"`
	Summary   string `xml:"summary"`
	Content   string `xml:"content"`
	Author    struct {
		Name  string `xml:"name"`
		Email string `xml:"email"`
	} `xml:"author"`
}

type atomXML struct {
	Title   string      `xml:"title"`
	Entries []atomEntry `xml:"entry"`
}

func atomLink(e atomEntry) string {
	for _, l := range e.Links {
		if l.Rel == "alternate" || l.Rel == "" {
			return l.Href
		}
	}
	return e.ID
}

func atomContent(e atomEntry) string {
	if e.Content != "" {
		return e.Content
	}
	return e.Summary
}

func atomPublished(e atomEntry) string {
	if e.Published != "" {
		return e.Published
	}
	return e.Updated
}

func parseAtomFeed(data []byte, now time.Time) (title string, articles []Article, err error) {
	var f atomXML
	if err := xml.Unmarshal(data, &f); err != nil {
		return "", nil, err
	}
	for _, e := range f.Entries {
		link := atomLink(e)
		content := sanitizer.Sanitize(resolveRelativeURLs(atomContent(e), link))
		articles = append(articles, Article{
			Link:        link,
			Title:       e.Title,
			Author:      e.Author.Name,
			AuthorEmail: e.Author.Email,
			PubDate:     parseDate(atomPublished(e)),
			Summary:     summarize(content, 220),
			Content:     content,
			FetchedAt:   now,
		})
	}
	return f.Title, articles, nil
}
