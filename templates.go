package main

import (
	"fmt"
	"html/template"
	"math"
	"strings"
	"time"
)

func relTime(t time.Time, short bool) string {
	if t.IsZero() {
		return ""
	}
	diffSec := time.Since(t).Seconds()
	switch {
	case diffSec < 3600:
		return fmt.Sprintf("%dm ago", int(math.Max(0, math.Floor(diffSec/60))))
	case diffSec < 86400:
		return fmt.Sprintf("%dh ago", int(diffSec/3600))
	case !short && diffSec < 604800:
		return fmt.Sprintf("%dd ago", int(diffSec/86400))
	default:
		return t.Format("Jan 2")
	}
}

func fullTime(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.Format("Mon, Jan 2, 2006")
}

func matchColor(ratio float64) template.CSS {
	switch {
	case ratio >= 0.9:
		return "var(--accent)"
	case ratio >= 0.6:
		return "var(--warn)"
	default:
		return "#555"
	}
}

func iconBtn(id, title, icon string) template.HTML {
	return template.HTML(fmt.Sprintf(`<button id="%s" class="icon-btn" data-icon="%s" title="%s"></button>`, id, icon, title))
}

// one shared shape for a settings stat tile
func statTile(id, label string) template.HTML {
	return template.HTML(fmt.Sprintf(`<div class="stat"><div id="%s" class="stat-value">0</div><div class="stat-label">%s</div></div>`, id, label))
}

var tmplFuncs = template.FuncMap{
	"relTime":    relTime,
	"fullTime":   fullTime,
	"matchColor": matchColor,
	"iconBtn":    iconBtn,
	"statTile":   statTile,
}

var tmpl = template.Must(template.New("root").Funcs(tmplFuncs).Parse(templates))

func render(name string, data any) (template.HTML, error) {
	var buf strings.Builder
	if err := tmpl.ExecuteTemplate(&buf, name, data); err != nil {
		return "", err
	}
	return template.HTML(buf.String()), nil
}

const templates = `
{{define "feedItem"}}
<div class="feed-item{{if .Active}} active{{end}}" data-feed-id="{{.ID}}">
	<span class="feed-name">{{.Title}}</span>
	{{if gt .Unread 0}}<span class="badge">{{if gt .Unread 99}}99+{{else}}{{.Unread}}{{end}}</span>{{end}}
	{{if .ID}}<button class="icon-btn refresh-btn" data-icon="refresh" data-action="refresh" title="Refresh"></button>
	<button class="icon-btn remove-btn" data-icon="x" data-action="remove" title="Remove feed"></button>{{end}}
</div>
{{end}}

{{define "feedList"}}
{{if not .}}<div class="feed-list-empty">No feeds yet.</div>{{else}}{{range .}}{{template "feedItem" .}}{{end}}{{end}}
{{end}}

{{define "articleRow"}}
<div class="article-row{{if not .Read}} unread{{end}}{{if .Selected}} selected{{end}}" data-article-id="{{.ID}}">
	{{if not .Read}}<span class="unread-dot"></span>{{end}}
	<div class="title">{{.Title}}</div>
	{{if .Summary}}<div class="summary">{{.Summary}}</div>{{end}}
	<div class="meta"><span>{{.FeedTitle}}</span>{{if not .PubDate.IsZero}}<span class="sep">&middot;</span><span>{{relTime .PubDate false}}</span>{{end}}</div>
</div>
{{end}}

{{define "articleList"}}
{{if not .}}<div class="list-empty">No articles</div>{{else}}{{range .}}{{template "articleRow" .}}{{end}}{{end}}
{{end}}

{{define "readerContent"}}
<h1 class="reader-title">{{.Title}}</h1>
<div class="reader-byline">
	{{if .Author}}<span class="author"><span>{{.Author}}</span>{{if .AuthorEmail}}<span class="author-email">{{.AuthorEmail}}</span>{{end}}</span>{{end}}
	{{if not .PubDate.IsZero}}<span>{{fullTime .PubDate}}</span>{{end}}
</div>
<div class="article-prose">{{.ContentHTML}}</div>
{{end}}

{{define "searchDynamic"}}
<div id="search-keywords" class="keyword-chips{{if not .Keywords}} hidden{{end}}">
	{{range .Keywords}}<span class="chip">{{.}}</span>{{end}}
	{{if .Keywords}}<span class="keyword-count">{{len .Keywords}} keyword{{if ne (len .Keywords) 1}}s{{end}}</span>{{end}}
</div>
<div id="search-results" class="scroll-hidden search-results">
	{{if not .Query}}<div class="search-empty">type to search across all {{.TotalArticles}} cached articles</div>
	{{else if not .Results}}<div class="search-empty">no results</div>
	{{else}}{{range $i, $r := .Results}}
		<div class="search-result-row" data-idx="{{$i}}" data-article-id="{{$r.Article.ID}}">
			<div class="title">{{$r.Article.Title}}</div>
			<div class="meta">
				<span>{{$r.Article.FeedTitle}}</span>
				{{if not $r.Article.PubDate.IsZero}}<span>&middot;</span><span>{{relTime $r.Article.PubDate true}}</span>{{end}}
				<span class="match-badge" style="color:{{matchColor $r.Ratio}}">{{$r.Matched}}/{{$r.Total}} matched</span>
			</div>
		</div>
	{{end}}{{end}}
</div>
{{end}}

{{define "sidebar"}}
<aside id="sidebar">
	<div class="sidebar-header">
		{{iconBtn "btn-search" "Search (/)" "search"}}
		<div class="sidebar-header-actions">
			{{iconBtn "btn-refresh-all" "Refresh all" "refresh"}}
			{{iconBtn "btn-settings" "Settings" "gear"}}
		</div>
	</div>
	<div id="feed-list" class="scroll-hidden feed-list">{{template "feedList" .Feeds}}</div>
	<div class="add-feed">
		<input id="add-feed-input" placeholder="Add feed URL&hellip;" autocomplete="off" />
		<button id="add-feed-btn" data-icon="plus" title="Add feed" aria-label="Add feed"></button>
	</div>
</aside>
{{end}}

{{define "articlePane"}}
<div id="article-pane">
	<div class="toolbar">
		<span id="feed-title-label">{{.FeedTitleLabel}}</span>
		<span id="feed-item-count" class="dim-label">{{.ItemCountLabel}}</span>
		<button id="mark-all-read-btn">Mark all read</button>
	</div>
	<div id="article-list" class="scroll-hidden">{{template "articleList" .Articles}}</div>
</div>
{{end}}

{{define "readerPanel"}}
<div id="reader" class="reader">
	<div class="reader-header">
		{{iconBtn "reader-close" "Close (Esc)" "chevron"}}
		<span id="reader-feed-title"></span>
		<a id="reader-open-original" class="icon-btn" data-icon="external" title="Open original article" target="_blank" rel="noopener"></a>
	</div>
	<div id="reader-content" class="scroll-hidden"></div>
</div>
{{end}}

{{define "searchDialog"}}
<dialog id="search-overlay" class="modal-dialog search-modal">
	<div class="search-bar">
		<span class="icon" data-icon="search"></span>
		<input id="search-input" placeholder='Search keywords or "exact phrases"' autocomplete="off" />
		<span id="search-count" class="dim-label"></span>
		<kbd>Esc</kbd>
	</div>
	<div class="search-filters">
		<label>From <input type="date" id="search-from" /></label>
		<label>To <input type="date" id="search-to" /></label>
	</div>
	<div id="search-dynamic">{{template "searchDynamic" .SearchInitial}}</div>
</dialog>
{{end}}

{{define "settingsDialog"}}
<dialog id="settings-overlay" class="modal-dialog settings-modal">
	<div class="modal-header"><span>Settings</span>{{iconBtn "settings-close" "" "x"}}</div>
	<div class="stats-row">{{statTile "stat-feeds" "Feeds"}}{{statTile "stat-articles" "Articles cached"}}</div>
	<div class="settings-section">
		<div class="section-label">OPML</div>
		<div class="action-row">
			<button id="btn-import" class="action-btn">Import OPML</button>
			<button id="btn-export" class="action-btn">Export OPML</button>
		</div>
		<input id="opml-file-input" type="file" accept=".opml,.xml" class="hidden" />
		<p id="opml-hint" class="hint">Import/export OPML 2.0</p>
	</div>
	<div class="settings-section">
		<div class="section-label">Feeds</div>
		<div class="action-row"><button id="btn-refresh-all-2" class="action-btn">Refresh all feeds</button></div>
		<label for="article-cap-input" class="section-label">Max articles per feed</label>
		<div class="action-row">
			<input id="article-cap-input" type="number" min="0" placeholder="Unlimited" />
			<button id="btn-save-cap" class="action-btn">Save</button>
		</div>
	</div>
	<div class="settings-section">
		<div class="section-label">Storage</div>
		<button id="btn-clear-cache" class="action-btn danger">Clear article cache</button>
		<p class="hint">Clears cached articles. Feeds are preserved.</p>
	</div>
</dialog>
{{end}}

{{define "shell"}}<!doctype html>
<html lang="en">
<head>
	<meta charset="utf-8" />
	<meta name="viewport" content="width=device-width, initial-scale=1" />
	<title>OpenRSS</title>
	<link rel="stylesheet" href="/style.css" />
</head>
<body>
	<div id="app">
		{{template "sidebar" .}}
		<div id="main">
			{{template "articlePane" .}}
			{{template "readerPanel" .}}
		</div>
	</div>

	{{template "searchDialog" .}}
	{{template "settingsDialog" .}}

	<script type="module" src="/app.js"></script>
</body>
</html>
{{end}}
`
