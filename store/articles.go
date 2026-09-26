package store

import (
	"strings"
	"time"
)

// keeps newest per feed; keep <= 0 means unlimited (no-op)
func (s *Store) PruneArticles(feedID int64, keep int) error {
	if keep <= 0 {
		return nil
	}
	_, err := s.db.Exec(
		`DELETE FROM articles WHERE feed_id = ? AND id NOT IN (
			SELECT id FROM articles WHERE feed_id = ? ORDER BY pub_date DESC, fetched_at DESC LIMIT ?
		)`, feedID, feedID, keep,
	)
	return err
}

// 0 means unlimited
func (s *Store) GetArticleCap() (int, error) {
	var articleCap int
	err := s.db.QueryRow(`SELECT article_cap FROM settings WHERE id = 1`).Scan(&articleCap)
	return articleCap, err
}

func (s *Store) SetArticleCap(articleCap int) error {
	_, err := s.db.Exec(`UPDATE settings SET article_cap = ? WHERE id = 1`, articleCap)
	return err
}

// prunes a feed's articles down to the currently configured cap
func (s *Store) PruneToCap(feedID int64) error {
	articleCap, err := s.GetArticleCap()
	if err != nil {
		return err
	}
	return s.PruneArticles(feedID, articleCap)
}

// no-op on duplicate link
func (s *Store) InsertArticle(a Article) (inserted bool, err error) {
	res, err := s.db.Exec(
		`INSERT OR IGNORE INTO articles(
			feed_id, link, title, author, author_email,
			pub_date, summary, content, fetched_at
		) VALUES(?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		a.FeedID, a.Link, a.Title, a.Author, a.AuthorEmail,
		formatTime(a.PubDate), a.Summary, a.Content, formatTime(a.FetchedAt),
	)
	if err != nil {
		return false, err
	}
	n, err := res.RowsAffected()
	return n > 0, err
}

// list, no content
func (s *Store) ListArticles(feedID int64) ([]Article, error) {
	query := `SELECT ` + articleColumns + ` FROM articles a JOIN feeds f ON f.id = a.feed_id`
	args := []any{}
	if feedID > 0 {
		query += ` WHERE a.feed_id = ?`
		args = append(args, feedID)
	}
	query += ` ORDER BY a.pub_date DESC`

	rows, err := s.db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanArticles(rows, false)
}

// marks read
func (s *Store) GetArticle(id int64) (Article, error) {
	row := s.db.QueryRow(
		`SELECT a.id, a.feed_id, f.title, a.link, a.title, a.author, a.author_email,
			a.pub_date, a.summary, a.content, a.fetched_at, a.read, a.full
		 FROM articles a JOIN feeds f ON f.id = a.feed_id WHERE a.id = ?`, id,
	)
	var a Article
	var pubDate, fetchedAt string
	err := row.Scan(
		&a.ID, &a.FeedID, &a.FeedTitle, &a.Link, &a.Title, &a.Author,
		&a.AuthorEmail, &pubDate, &a.Summary, &a.Content,
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

// fts-narrowed candidates for search.go's ranking pass, newest match first
func (s *Store) SearchCandidates(keywords []string, from, to time.Time) ([]Article, error) {
	parts := make([]string, len(keywords))
	for i, kw := range keywords {
		parts[i] = `"` + strings.ReplaceAll(kw, `"`, `""`) + `"`
	}
	match := strings.Join(parts, " OR ")

	query := `SELECT ` + articleColumnsWithContent + `
		FROM articles_fts
		JOIN articles a ON a.id = articles_fts.rowid
		JOIN feeds f ON f.id = a.feed_id
		WHERE articles_fts MATCH ?`
	args := []any{match}
	if !from.IsZero() {
		query += ` AND a.pub_date >= ?`
		args = append(args, formatTime(from))
	}
	if !to.IsZero() {
		query += ` AND a.pub_date < ?`
		args = append(args, formatTime(to))
	}
	query += ` ORDER BY bm25(articles_fts)`

	rows, err := s.db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanArticles(rows, true)
}
