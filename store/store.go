package store

import (
	"database/sql"
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
	ID          int64     `json:"id"`
	FeedID      int64     `json:"feedId"`
	FeedTitle   string    `json:"feedTitle"`
	Link        string    `json:"link"`
	Title       string    `json:"title"`
	Author      string    `json:"author,omitempty"`
	AuthorEmail string    `json:"authorEmail,omitempty"`
	PubDate     time.Time `json:"pubDate,omitempty"`
	Summary     string    `json:"summary"`
	Content     string    `json:"content,omitempty"`
	FetchedAt   time.Time `json:"fetchedAt"`
	Read        bool      `json:"read"`
	Full        bool      `json:"-"`
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
CREATE TABLE IF NOT EXISTS settings (
	id          INTEGER PRIMARY KEY CHECK (id = 1),
	article_cap INTEGER NOT NULL DEFAULT 0
);
INSERT OR IGNORE INTO settings (id, article_cap) VALUES (1, 0);

CREATE VIRTUAL TABLE IF NOT EXISTS articles_fts USING fts5(
	title, summary, content,
	content='articles', content_rowid='id'
);
CREATE TRIGGER IF NOT EXISTS articles_ai AFTER INSERT ON articles BEGIN
	INSERT INTO articles_fts(rowid, title, summary, content) VALUES (new.id, new.title, new.summary, new.content);
END;
CREATE TRIGGER IF NOT EXISTS articles_ad AFTER DELETE ON articles BEGIN
	INSERT INTO articles_fts(articles_fts, rowid, title, summary, content) VALUES('delete', old.id, old.title, old.summary, old.content);
END;
CREATE TRIGGER IF NOT EXISTS articles_au AFTER UPDATE ON articles BEGIN
	INSERT INTO articles_fts(articles_fts, rowid, title, summary, content) VALUES('delete', old.id, old.title, old.summary, old.content);
	INSERT INTO articles_fts(rowid, title, summary, content) VALUES (new.id, new.title, new.summary, new.content);
END;
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

// backfills the fts index for dbs that had articles before it existed
func migrate(db *sql.DB) error {
	var ftsCount, articleCount int
	if err := db.QueryRow(`SELECT count(*) FROM articles_fts`).Scan(&ftsCount); err != nil {
		return err
	}
	if err := db.QueryRow(`SELECT count(*) FROM articles`).Scan(&articleCount); err != nil {
		return err
	}
	if ftsCount == 0 && articleCount > 0 {
		if _, err := db.Exec(`INSERT INTO articles_fts(articles_fts) VALUES('rebuild')`); err != nil {
			return err
		}
	}
	return nil
}

func (s *Store) Close() error { return s.db.Close() }

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

func formatTime(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.UTC().Format(time.RFC3339)
}
