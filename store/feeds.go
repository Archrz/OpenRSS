package store

import "database/sql"

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

func (s *Store) GetFeed(id int64) (Feed, error) {
	var f Feed
	err := s.db.QueryRow(`SELECT id, url, title, site_url, etag, last_modified FROM feeds WHERE id = ?`, id).
		Scan(&f.ID, &f.URL, &f.Title, &f.SiteURL, &f.ETag, &f.LastModified)
	return f, err
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
