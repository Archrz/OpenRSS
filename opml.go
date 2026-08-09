package main

import (
	"encoding/xml"
	"fmt"
	"log"
	"strings"
	"time"
)

type opmlOutline struct {
	Text     string        `xml:"text,attr"`
	Title    string        `xml:"title,attr"`
	XMLURL   string        `xml:"xmlUrl,attr"`
	HTMLURL  string        `xml:"htmlUrl,attr"`
	Outlines []opmlOutline `xml:"outline"`
}

type opmlDoc struct {
	Body struct {
		Outlines []opmlOutline `xml:"outline"`
	} `xml:"body"`
}

// includes nested outlines
func ParseOPML(data []byte) ([]Feed, error) {
	var doc opmlDoc
	if err := xml.Unmarshal(data, &doc); err != nil {
		return nil, err
	}
	feeds := []Feed{}
	var walk func([]opmlOutline)
	walk = func(outlines []opmlOutline) {
		for _, o := range outlines {
			if o.XMLURL != "" {
				title := o.Title
				if title == "" {
					title = o.Text
				}
				feeds = append(feeds, Feed{URL: o.XMLURL, Title: title, SiteURL: o.HTMLURL})
			}
			walk(o.Outlines)
		}
	}
	walk(doc.Body.Outlines)
	return feeds, nil
}

func ImportOPMLData(s *Store, data []byte) (int, error) {
	feeds, err := ParseOPML(data)
	if err != nil {
		return 0, err
	}
	imported := 0
	for _, f := range feeds {
		id, err := s.AddFeed(f.URL, f.Title, f.SiteURL)
		if err != nil {
			log.Printf("import feed %s: %v", f.URL, err)
			continue
		}
		imported++
		go RefreshFeed(s, Feed{ID: id, URL: f.URL, Title: f.Title})
	}
	return imported, nil
}

func ExportOPML(feeds []Feed) string {
	var items strings.Builder
	for _, f := range feeds {
		fmt.Fprintf(&items, `    <outline type="rss" text="%s" title="%s" xmlUrl="%s"`,
			escXML(f.Title), escXML(f.Title), escXML(f.URL))
		if f.SiteURL != "" {
			fmt.Fprintf(&items, ` htmlUrl="%s"`, escXML(f.SiteURL))
		}
		items.WriteString("/>\n")
	}
	return `<?xml version="1.0" encoding="UTF-8"?>
<opml version="2.0">
  <head>
    <title>OpenRSS subscriptions</title>
    <dateCreated>` + time.Now().UTC().Format(time.RFC1123Z) + `</dateCreated>
  </head>
  <body>
    <outline text="Feeds" title="Feeds">
` + items.String() + `    </outline>
  </body>
</opml>
`
}

func escXML(s string) string {
	r := strings.NewReplacer("&", "&amp;", `"`, "&quot;", "<", "&lt;", ">", "&gt;")
	return r.Replace(s)
}
