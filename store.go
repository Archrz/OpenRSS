package main

import (
	"database/sql"
	"strings"
	"time"

	_ "modernc.org/sqlite"
)

type Feed struct {
	ID           int64  `json:"id"`
	URL          string `json:"url"`
	Title        string `json:"title"`
	SiteURL      string `json:"siteUrl,omitempty"`
	ETag         string `json:"-"`
	LastModified string `json:"-"`
}

type Article struct {
	ID           int64     `json:"id"`
	FeedID       int64     `json:"feedId"`
	FeedTitle    string    `json:"feedTitle"`
	Link         string    `json:"link"`
	Title        string    `json:"title"`
	Author       string    `json:"author,omitempty"`
	AuthorEmail  string    `json:"authorEmail,omitempty"`
	AuthorAvatar string    `json:"authorAvatar,omitempty"`
	PubDate      time.Time `json:"pubDate,omitempty"`
	Summary      string    `json:"summary"`
	Content      string    `json:"content,omitempty"`
	FetchedAt    time.Time `json:"fetchedAt"`
	Read         bool      `json:"read"`
	Full         bool      `json:"-"`
}

type Store struct {
	db *sql.DB
}

const schema = `
CREATE TABLE IF NOT EXISTS feeds (
	id            INTEGER PRIMARY KEY,
	url           TEXT UNIQUE NOT NULL,
	title         TEXT NOT NULL,
	site_url      TEXT NOT NULL DEFAULT '',
	etag          TEXT NOT NULL DEFAULT '',
	last_modified TEXT NOT NULL DEFAULT ''
);
CREATE TABLE IF NOT EXISTS articles (
	id            INTEGER PRIMARY KEY,
	feed_id       INTEGER NOT NULL REFERENCES feeds(id) ON DELETE CASCADE,
	link          TEXT NOT NULL,
	title         TEXT NOT NULL,
	author        TEXT NOT NULL DEFAULT '',
	author_email  TEXT NOT NULL DEFAULT '',
	author_avatar TEXT NOT NULL DEFAULT '',
	pub_date      TEXT NOT NULL DEFAULT '',
	summary       TEXT NOT NULL DEFAULT '',
	content       TEXT NOT NULL DEFAULT '',
	fetched_at    TEXT NOT NULL,
	read          INTEGER NOT NULL DEFAULT 0,
	full          INTEGER NOT NULL DEFAULT 0,
	UNIQUE(feed_id, link)
);
CREATE INDEX IF NOT EXISTS idx_articles_feed ON articles(feed_id);
CREATE INDEX IF NOT EXISTS idx_articles_pubdate ON articles(pub_date);
`

// single connection avoids SQLITE_BUSY
func OpenStore(path string) (*Store, error) {
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)
	if _, err := db.Exec("PRAGMA foreign_keys = ON"); err != nil {
		db.Close()
		return nil, err
	}
	if _, err := db.Exec(schema); err != nil {
		db.Close()
		return nil, err
	}
	if err := migrate(db); err != nil {
		db.Close()
		return nil, err
	}
	return &Store{db: db}, nil
}

// migrate existing dbs
func migrate(db *sql.DB) error {
	alters := []string{
		`ALTER TABLE feeds ADD COLUMN etag TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE feeds ADD COLUMN last_modified TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE articles ADD COLUMN full INTEGER NOT NULL DEFAULT 0`,
		`ALTER TABLE articles ADD COLUMN author_email TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE articles ADD COLUMN author_avatar TEXT NOT NULL DEFAULT ''`,
	}
	for _, stmt := range alters {
		if _, err := db.Exec(stmt); err != nil && !strings.Contains(err.Error(), "duplicate column") {
			return err
		}
	}
	return nil
}

func (s *Store) Close() error { return s.db.Close() }

func (s *Store) ListFeeds() ([]Feed, error) {
	rows, err := s.db.Query(`SELECT id, url, title, site_url, etag, last_modified FROM feeds ORDER BY title`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	feeds := []Feed{}
	for rows.Next() {
		var f Feed
		if err := rows.Scan(&f.ID, &f.URL, &f.Title, &f.SiteURL, &f.ETag, &f.LastModified); err != nil {
			return nil, err
		}
		feeds = append(feeds, f)
	}
	return feeds, rows.Err()
}

func (s *Store) AddFeed(url, title, siteURL string) (int64, error) {
	var id int64
	err := s.db.QueryRow(`SELECT id FROM feeds WHERE url = ?`, url).Scan(&id)
	if err == nil {
		return id, nil
	}
	if err != sql.ErrNoRows {
		return 0, err
	}
	res, err := s.db.Exec(`INSERT INTO feeds(url, title, site_url) VALUES(?, ?, ?)`, url, title, siteURL)
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

func (s *Store) SetFeedTitle(id int64, title string) error {
	_, err := s.db.Exec(`UPDATE feeds SET title = ? WHERE id = ?`, title, id)
	return err
}

// enables conditional GET
func (s *Store) SetFeedCache(id int64, etag, lastModified string) error {
	_, err := s.db.Exec(`UPDATE feeds SET etag = ?, last_modified = ? WHERE id = ?`, etag, lastModified, id)
	return err
}

func (s *Store) RemoveFeed(id int64) error {
	_, err := s.db.Exec(`DELETE FROM feeds WHERE id = ?`, id)
	return err
}

// keeps newest per feed
func (s *Store) PruneArticles(feedID int64, keep int) error {
	_, err := s.db.Exec(
		`DELETE FROM articles WHERE feed_id = ? AND id NOT IN (
			SELECT id FROM articles WHERE feed_id = ? ORDER BY pub_date DESC, fetched_at DESC LIMIT ?
		)`, feedID, feedID, keep,
	)
	return err
}

// no-op on duplicate link
func (s *Store) InsertArticle(a Article) (inserted bool, err error) {
	res, err := s.db.Exec(
		`INSERT OR IGNORE INTO articles(
			feed_id, link, title, author, author_email, author_avatar,
			pub_date, summary, content, fetched_at
		) VALUES(?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		a.FeedID, a.Link, a.Title, a.Author, a.AuthorEmail, a.AuthorAvatar,
		formatTime(a.PubDate), a.Summary, a.Content, formatTime(a.FetchedAt),
	)
	if err != nil {
		return false, err
	}
	n, err := res.RowsAffected()
	return n > 0, err
}

const (
	articleColumns = `a.id, a.feed_id, f.title, a.link, a.title, a.author,
		a.pub_date, a.summary, a.fetched_at, a.read`
	articleColumnsWithContent = `a.id, a.feed_id, f.title, a.link, a.title, a.author,
		a.pub_date, a.summary, a.content, a.fetched_at, a.read`
)

// scans either column set
func scanArticles(rows *sql.Rows, withContent bool) ([]Article, error) {
	articles := []Article{}
	for rows.Next() {
		var a Article
		var pubDate, fetchedAt string
		dest := []any{&a.ID, &a.FeedID, &a.FeedTitle, &a.Link, &a.Title, &a.Author, &pubDate, &a.Summary}
		if withContent {
			dest = append(dest, &a.Content)
		}
		dest = append(dest, &fetchedAt, &a.Read)
		if err := rows.Scan(dest...); err != nil {
			return nil, err
		}
		a.PubDate, _ = time.Parse(time.RFC3339, pubDate)
		a.FetchedAt, _ = time.Parse(time.RFC3339, fetchedAt)
		articles = append(articles, a)
	}
	return articles, rows.Err()
}

// list, no content
func (s *Store) ListArticles(feedID int64) ([]Article, error) {
	query := `SELECT ` + articleColumns + ` FROM articles a JOIN feeds f ON f.id = a.feed_id`
	var rows *sql.Rows
	var err error
	if feedID > 0 {
		rows, err = s.db.Query(query+` WHERE a.feed_id = ? ORDER BY a.pub_date DESC`, feedID)
	} else {
		rows, err = s.db.Query(query + ` ORDER BY a.pub_date DESC`)
	}
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanArticles(rows, false)
}

// marks read
func (s *Store) GetArticle(id int64) (Article, error) {
	row := s.db.QueryRow(
		`SELECT a.id, a.feed_id, f.title, a.link, a.title, a.author, a.author_email, a.author_avatar,
			a.pub_date, a.summary, a.content, a.fetched_at, a.read, a.full
		 FROM articles a JOIN feeds f ON f.id = a.feed_id WHERE a.id = ?`, id,
	)
	var a Article
	var pubDate, fetchedAt string
	err := row.Scan(
		&a.ID, &a.FeedID, &a.FeedTitle, &a.Link, &a.Title, &a.Author,
		&a.AuthorEmail, &a.AuthorAvatar, &pubDate, &a.Summary, &a.Content,
		&fetchedAt, &a.Read, &a.Full,
	)
	if err != nil {
		return Article{}, err
	}
	a.PubDate, _ = time.Parse(time.RFC3339, pubDate)
	a.FetchedAt, _ = time.Parse(time.RFC3339, fetchedAt)
	if !a.Read {
		if _, err := s.db.Exec(`UPDATE articles SET read = 1 WHERE id = ?`, id); err != nil {
			return Article{}, err
		}
		a.Read = true
	}
	return a, nil
}

func (s *Store) SetArticleContent(id int64, content string) error {
	_, err := s.db.Exec(`UPDATE articles SET content = ?, full = 1 WHERE id = ?`, content, id)
	return err
}

func (s *Store) MarkArticleFull(id int64) error {
	_, err := s.db.Exec(`UPDATE articles SET full = 1 WHERE id = ?`, id)
	return err
}

func (s *Store) SetArticleAuthorAvatar(id int64, avatar string) error {
	_, err := s.db.Exec(`UPDATE articles SET author_avatar = ? WHERE id = ?`, avatar, id)
	return err
}

func (s *Store) MarkAllRead(feedID int64) error {
	if feedID > 0 {
		_, err := s.db.Exec(`UPDATE articles SET read = 1 WHERE feed_id = ?`, feedID)
		return err
	}
	_, err := s.db.Exec(`UPDATE articles SET read = 1`)
	return err
}

// feeds untouched
func (s *Store) ClearArticles() error {
	_, err := s.db.Exec(`DELETE FROM articles`)
	return err
}

func (s *Store) Stats() (feedCount, articleCount int, err error) {
	if err = s.db.QueryRow(`SELECT COUNT(*) FROM feeds`).Scan(&feedCount); err != nil {
		return 0, 0, err
	}
	err = s.db.QueryRow(`SELECT COUNT(*) FROM articles`).Scan(&articleCount)
	return feedCount, articleCount, err
}

// with content, for search
func (s *Store) ListArticlesWithContent() ([]Article, error) {
	rows, err := s.db.Query(`SELECT ` + articleColumnsWithContent + ` FROM articles a JOIN feeds f ON f.id = a.feed_id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanArticles(rows, true)
}

func formatTime(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.UTC().Format(time.RFC3339)
}
