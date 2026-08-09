package main

import (
	"crypto/sha256"
	"encoding/hex"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	xhtml "golang.org/x/net/html"
)

// empty disables caching
var imageCacheDir string

var hashRE = regexp.MustCompile(`^[0-9a-f]{64}$`)

// img src to local cache, keeps original on failure
func cacheArticleImages(rawHTML string) string {
	if imageCacheDir == "" {
		return rawHTML
	}
	z := xhtml.NewTokenizer(strings.NewReader(rawHTML))
	var out strings.Builder
	for {
		if z.Next() == xhtml.ErrorToken {
			break
		}
		tok := z.Token()
		if tok.Data == "img" && (tok.Type == xhtml.StartTagToken || tok.Type == xhtml.SelfClosingTagToken) {
			for i, a := range tok.Attr {
				if a.Key == "src" {
					if local, ok := cacheImage(a.Val); ok {
						tok.Attr[i].Val = local
					}
				}
			}
		}
		out.WriteString(tok.String())
	}
	return out.String()
}

// false if disabled or not http(s)
func cacheImage(srcURL string) (string, bool) {
	if imageCacheDir == "" {
		return "", false
	}
	if !strings.HasPrefix(srcURL, "http://") && !strings.HasPrefix(srcURL, "https://") {
		return "", false
	}
	sum := sha256.Sum256([]byte(srcURL))
	hash := hex.EncodeToString(sum[:])
	localPath := filepath.Join(imageCacheDir, hash)
	localURL := "/api/images/" + hash

	if _, err := os.Stat(localPath); err == nil {
		return localURL, true
	}

	req, err := http.NewRequest(http.MethodGet, srcURL, nil)
	if err != nil {
		return "", false
	}
	req.Header.Set("User-Agent", "OpenRSS/1.0 (RSS reader)")
	resp, err := httpClient.Do(req)
	if err != nil {
		return "", false
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", false
	}

	tmp := localPath + ".tmp"
	f, err := os.Create(tmp)
	if err != nil {
		return "", false
	}
	_, err = io.Copy(f, io.LimitReader(resp.Body, 20<<20)) // cap at 20MB
	f.Close()
	if err != nil {
		os.Remove(tmp)
		return "", false
	}
	if err := os.Rename(tmp, localPath); err != nil {
		os.Remove(tmp)
		return "", false
	}
	return localURL, true
}

// sniffs content-type, hash has no extension
func imagesHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		hash := r.PathValue("hash")
		if !hashRE.MatchString(hash) {
			http.NotFound(w, r)
			return
		}
		f, err := os.Open(filepath.Join(imageCacheDir, hash))
		if err != nil {
			http.NotFound(w, r)
			return
		}
		defer f.Close()
		info, err := f.Stat()
		if err != nil {
			http.NotFound(w, r)
			return
		}
		http.ServeContent(w, r, "", info.ModTime(), f)
	}
}
