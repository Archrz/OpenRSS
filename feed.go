package main

import (
	"bytes"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"html"
	"io"
	"log"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"

	readability "codeberg.org/readeck/go-readability/v2"
	"github.com/microcosm-cc/bluemonday"
	xhtml "golang.org/x/net/html"
)

type rssXML struct {
	Channel struct {
		Title string `xml:"title"`
		Items []struct {
			Title       string `xml:"title"`
			Link        string `xml:"link"`
			GUID        string `xml:"guid"`
			PubDate     string `xml:"pubDate"`
			Creator     string `xml:"creator"` // dc:creator, namespace-agnostic match
			Author      string `xml:"author"`
			Description string `xml:"description"`
			Encoded     string `xml:"encoded"` // content:encoded, namespace-agnostic match
		} `xml:"item"`
	} `xml:"channel"`
}

type atomXML struct {
	Title   string `xml:"title"`
	Entries []struct {
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
	} `xml:"entry"`
}

var dateLayouts = []string{
	time.RFC1123Z, time.RFC1123, time.RFC3339, time.RFC822Z, time.RFC822,
	"Mon, 2 Jan 2006 15:04:05 -0700", "2006-01-02T15:04:05Z",
}

func parseDate(s string) time.Time {
	s = strings.TrimSpace(s)
	for _, layout := range dateLayouts {
		if t, err := time.Parse(layout, s); err == nil {
			return t
		}
	}
	return time.Time{}
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

// JSON Feed 1.1 (jsonfeed.org).
type jsonFeed struct {
	Title string `json:"title"`
	Items []struct {
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
			Name   string `json:"name"` // JSON Feed 1.0's singular (deprecated) form
			Avatar string `json:"avatar"`
		} `json:"author"`
	} `json:"items"`
}

func parseJSONFeed(data []byte, fetchedAt time.Time) (title string, articles []Article, err error) {
	var f jsonFeed
	if err := json.Unmarshal(data, &f); err != nil {
		return "", nil, err
	}
	for _, it := range f.Items {
		rawContent := it.ContentHTML
		if rawContent == "" {
			rawContent = it.ContentText
		}
		if rawContent == "" {
			rawContent = it.Summary
		}
		link := it.URL
		if link == "" {
			link = it.ID
		}
		author, avatar := it.Author.Name, it.Author.Avatar
		if len(it.Authors) > 0 && it.Authors[0].Name != "" {
			author, avatar = it.Authors[0].Name, it.Authors[0].Avatar
		}
		if base, err := url.Parse(link); err == nil && base.IsAbs() {
			avatar = resolveURL(avatar, base)
		}
		published := it.DatePublished
		if published == "" {
			published = it.DateModified
		}
		content := sanitizer.Sanitize(resolveRelativeURLs(rawContent, link))
		articles = append(articles, Article{
			Link:         link,
			Title:        it.Title,
			Author:       author,
			AuthorAvatar: avatar,
			PubDate:      parseDate(published),
			Summary:      summarize(content, 220),
			Content:      content,
			FetchedAt:    fetchedAt,
		})
	}
	return f.Title, articles, nil
}

var sanitizer = bluemonday.UGCPolicy()

func resolveURL(rawURL string, base *url.URL) string {
	ref, err := url.Parse(rawURL)
	if err != nil || ref.IsAbs() || base == nil {
		return rawURL
	}
	return base.ResolveReference(ref).String()
}

// relative img/href to absolute
func resolveRelativeURLs(rawHTML, articleLink string) string {
	base, err := url.Parse(articleLink)
	if err != nil || !base.IsAbs() {
		return rawHTML
	}
	z := xhtml.NewTokenizer(strings.NewReader(rawHTML))
	var out strings.Builder
	for {
		if z.Next() == xhtml.ErrorToken {
			break
		}
		tok := z.Token()
		if tok.Type == xhtml.StartTagToken || tok.Type == xhtml.SelfClosingTagToken {
			for i, a := range tok.Attr {
				if a.Key == "src" || a.Key == "href" {
					tok.Attr[i].Val = resolveURL(a.Val, base)
				}
			}
		}
		out.WriteString(tok.String())
	}
	return out.String()
}

// one hung feed shouldn't stall the rest
var httpClient = &http.Client{Timeout: 20 * time.Second}

var (
	tagRE      = regexp.MustCompile(`<[^>]*>`)
	blankRunRE = regexp.MustCompile(`\s+`)
)

// HTML to plain-text snippet
func summarize(rawHTML string, max int) string {
	text := strings.TrimSpace(blankRunRE.ReplaceAllString(html.UnescapeString(tagRE.ReplaceAllString(rawHTML, " ")), " "))
	r := []rune(text)
	if len(r) > max {
		return string(r[:max]) + "…"
	}
	return text
}

type FeedFetchResult struct {
	Title        string
	Articles     []Article
	ETag         string
	LastModified string
	NotModified  bool
}

// RSS/Atom/JSON Feed, conditional GET
func FetchFeed(feedURL, etag, lastModified string) (FeedFetchResult, error) {
	req, err := http.NewRequest(http.MethodGet, feedURL, nil)
	if err != nil {
		return FeedFetchResult{}, err
	}
	req.Header.Set("User-Agent", "OpenRSS/1.0 (RSS reader)")
	if etag != "" {
		req.Header.Set("If-None-Match", etag)
	}
	if lastModified != "" {
		req.Header.Set("If-Modified-Since", lastModified)
	}
	resp, err := httpClient.Do(req)
	if err != nil {
		return FeedFetchResult{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusNotModified {
		return FeedFetchResult{NotModified: true}, nil
	}
	if resp.StatusCode != http.StatusOK {
		return FeedFetchResult{}, fmt.Errorf("%s: status %s", feedURL, resp.Status)
	}
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return FeedFetchResult{}, err
	}

	result := FeedFetchResult{ETag: resp.Header.Get("ETag"), LastModified: resp.Header.Get("Last-Modified")}
	now := time.Now()

	if trimmed := bytes.TrimSpace(data); len(trimmed) > 0 && trimmed[0] == '{' {
		title, articles, err := parseJSONFeed(trimmed, now)
		if err != nil {
			return FeedFetchResult{}, err
		}
		result.Title = title
		result.Articles = articles
		return result, nil
	}

	root, err := rootElement(data)
	if err != nil {
		return FeedFetchResult{}, err
	}

	switch root {
	case "rss":
		var f rssXML
		if err := xml.Unmarshal(data, &f); err != nil {
			return FeedFetchResult{}, err
		}
		for _, it := range f.Channel.Items {
			rawContent := it.Encoded
			if rawContent == "" {
				rawContent = it.Description
			}
			link := it.Link
			if link == "" {
				link = it.GUID
			}
			author, email := "", ""
			if it.Author != "" {
				author, email = parseRSSAuthor(it.Author)
			}
			if it.Creator != "" {
				author = it.Creator
			}
			content := sanitizer.Sanitize(resolveRelativeURLs(rawContent, link))
			result.Articles = append(result.Articles, Article{
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
		result.Title = f.Channel.Title
		return result, nil

	case "feed":
		var f atomXML
		if err := xml.Unmarshal(data, &f); err != nil {
			return FeedFetchResult{}, err
		}
		for _, e := range f.Entries {
			link := e.ID
			for _, l := range e.Links {
				if l.Rel == "alternate" || l.Rel == "" {
					link = l.Href
					break
				}
			}
			rawContent := e.Content
			if rawContent == "" {
				rawContent = e.Summary
			}
			published := e.Published
			if published == "" {
				published = e.Updated
			}
			content := sanitizer.Sanitize(resolveRelativeURLs(rawContent, link))
			result.Articles = append(result.Articles, Article{
				Link:        link,
				Title:       e.Title,
				Author:      e.Author.Name,
				AuthorEmail: e.Author.Email,
				PubDate:     parseDate(published),
				Summary:     summarize(content, 220),
				Content:     content,
				FetchedAt:   now,
			})
		}
		result.Title = f.Title
		return result, nil
	}
	return FeedFetchResult{}, fmt.Errorf("%s: unrecognized feed format <%s>", feedURL, root)
}

type RefreshResult struct {
	FeedID int64  `json:"feedId"`
	Added  int    `json:"added"`
	Error  string `json:"error,omitempty"`
}

// readability-extracts the live page
func FetchFullArticle(link string) (string, error) {
	article, err := readability.FromURL(link, 20*time.Second, func(r *http.Request) {
		r.Header.Set("User-Agent", "OpenRSS/1.0 (RSS reader)")
	})
	if err != nil {
		return "", err
	}
	var buf bytes.Buffer
	if err := article.RenderHTML(&buf); err != nil {
		return "", err
	}
	return sanitizer.Sanitize(buf.String()), nil
}

func RefreshAll(s *Store) []RefreshResult {
	feeds, err := s.ListFeeds()
	if err != nil {
		log.Printf("refresh: list feeds: %v", err)
		return nil
	}
	results := make([]RefreshResult, len(feeds))
	for i, f := range feeds {
		added, err := RefreshFeed(s, f)
		results[i] = RefreshResult{FeedID: f.ID, Added: added}
		if err != nil {
			results[i].Error = err.Error()
		}
	}
	return results
}

// per-feed history cap
const articlesPerFeedCap = 300

// returns new-article count
func RefreshFeed(s *Store, f Feed) (added int, err error) {
	result, err := FetchFeed(f.URL, f.ETag, f.LastModified)
	if err != nil {
		log.Printf("refresh %s: %v", f.URL, err)
		return 0, err
	}
	if result.NotModified {
		return 0, nil
	}
	if result.Title != "" && result.Title != f.Title && f.Title == f.URL {
		_ = s.SetFeedTitle(f.ID, result.Title)
	}
	for _, a := range result.Articles {
		if a.Link == "" {
			continue
		}
		a.FeedID = f.ID
		inserted, err := s.InsertArticle(a)
		if err != nil {
			log.Printf("refresh %s: store article %q: %v", f.URL, a.Title, err)
			continue
		}
		if inserted {
			added++
		}
	}
	if err := s.SetFeedCache(f.ID, result.ETag, result.LastModified); err != nil {
		log.Printf("refresh %s: save cache validators: %v", f.URL, err)
	}
	if err := s.PruneArticles(f.ID, articlesPerFeedCap); err != nil {
		log.Printf("refresh %s: prune articles: %v", f.URL, err)
	}
	return added, nil
}

func rootElement(data []byte) (string, error) {
	dec := xml.NewDecoder(strings.NewReader(string(data)))
	for {
		tok, err := dec.Token()
		if err != nil {
			return "", err
		}
		if se, ok := tok.(xml.StartElement); ok {
			return se.Name.Local, nil
		}
	}
}
