package feed

import (
	"bytes"
	"encoding/xml"
	"fmt"
	"html"
	"io"
	"log"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"

	readability "codeberg.org/readeck/go-readability/v2"
	"github.com/microcosm-cc/bluemonday"
	xhtml "golang.org/x/net/html"

	"openrss/store"
)

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

var sanitizer = bluemonday.UGCPolicy().AllowElements(
	"table", "thead", "tbody", "tfoot", "tr", "th", "td", "caption", "colgroup", "col",
)

// remove small avatar icons of author.
const maxIconPx = 150

func isIconSized(attrs []xhtml.Attribute) bool {
	var w, h int
	for _, a := range attrs {
		switch a.Key {
		case "width":
			w, _ = strconv.Atoi(a.Val)
		case "height":
			h, _ = strconv.Atoi(a.Val)
		}
	}
	return w > 0 && w <= maxIconPx && h > 0 && h <= maxIconPx
}

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
			if tok.Data == "img" && isIconSized(tok.Attr) {
				continue
			}
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

const userAgent = "OpenRSS/1.0 (RSS reader)"

// one hung feed shouldn't stall the rest
var httpClient = &http.Client{Timeout: 20 * time.Second}

var (
	tagRE      = regexp.MustCompile(`<[^>]*>`)
	blankRunRE = regexp.MustCompile(`\s+`)
)

// HTML to plain-text snippet
func summarize(rawHTML string, max int) string {
	noTags := tagRE.ReplaceAllString(rawHTML, " ")
	unescaped := html.UnescapeString(noTags)
	collapsed := blankRunRE.ReplaceAllString(unescaped, " ")
	text := strings.TrimSpace(collapsed)

	r := []rune(text)
	if len(r) > max {
		return string(r[:max]) + "…"
	}
	return text
}

// shared by the three format parsers: sanitize, summarize, resolve links
func buildArticle(link, title, author, email, pubDate, rawContent string, now time.Time) store.Article {
	content := sanitizer.Sanitize(resolveRelativeURLs(rawContent, link))
	return store.Article{
		Link:        link,
		Title:       title,
		Author:      author,
		AuthorEmail: email,
		PubDate:     parseDate(pubDate),
		Summary:     summarize(content, 220),
		Content:     content,
		FetchedAt:   now,
	}
}

type FeedFetchResult struct {
	Title        string
	Articles     []store.Article
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
	req.Header.Set("User-Agent", userAgent)
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

	parse, err := feedParser(data)
	if err != nil {
		return FeedFetchResult{}, fmt.Errorf("%s: %w", feedURL, err)
	}
	title, articles, err := parse(data, time.Now())
	if err != nil {
		return FeedFetchResult{}, err
	}
	return FeedFetchResult{
		Title:        title,
		Articles:     articles,
		ETag:         resp.Header.Get("ETag"),
		LastModified: resp.Header.Get("Last-Modified"),
	}, nil
}

// picks the right format parser
func feedParser(data []byte) (func([]byte, time.Time) (string, []store.Article, error), error) {
	if trimmed := bytes.TrimSpace(data); len(trimmed) > 0 && trimmed[0] == '{' {
		return parseJSONFeed, nil
	}
	root, err := rootElement(data)
	if err != nil {
		return nil, err
	}
	switch root {
	case "rss":
		return parseRSSFeed, nil
	case "feed":
		return parseAtomFeed, nil
	}
	return nil, fmt.Errorf("unrecognized feed format <%s>", root)
}

type RefreshResult struct {
	FeedID int64  `json:"feedId"`
	Added  int    `json:"added"`
	Error  string `json:"error,omitempty"`
}

// readability-extracts the live page
func FetchFullArticle(link string) (string, error) {
	article, err := readability.FromURL(link, httpClient.Timeout, func(r *http.Request) {
		r.Header.Set("User-Agent", userAgent)
	})
	if err != nil {
		return "", err
	}
	var buf bytes.Buffer
	if err := article.RenderHTML(&buf); err != nil {
		return "", err
	}
	return sanitizer.Sanitize(resolveRelativeURLs(buf.String(), link)), nil
}

func RefreshAll(s *store.Store) []RefreshResult {
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

// returns new-article count
func RefreshFeed(s *store.Store, f store.Feed) (added int, err error) {
	result, err := FetchFeed(f.URL, f.ETag, f.LastModified)
	if err != nil {
		log.Printf("refresh %s: %v", f.URL, err)
		return 0, err
	}
	if result.NotModified {
		return 0, nil
	}
	if result.Title != "" && result.Title != f.Title && f.Title == f.URL {
		if err := s.SetFeedTitle(f.ID, result.Title); err != nil {
			log.Printf("refresh %s: set feed title: %v", f.URL, err)
		}
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
	if err := s.PruneToCap(f.ID); err != nil {
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
