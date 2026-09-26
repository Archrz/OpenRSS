package main

import (
	"flag"
	"fmt"
	"log"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	webview "github.com/webview/webview_go"

	"openrss/feed"
	"openrss/store"
)

// stable path regardless of cwd
func defaultDBPath() string {
	dataHome := os.Getenv("XDG_DATA_HOME")
	if dataHome == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return "openrss.db"
		}
		dataHome = filepath.Join(home, ".local", "share")
	}
	return filepath.Join(dataHome, "openrss", "openrss.db")
}

func main() {
	dbPath := flag.String("db", defaultDBPath(), "sqlite database path")
	refresh := flag.Duration("refresh", 30*time.Minute, "feed refresh interval")
	flag.Parse()

	if err := os.MkdirAll(filepath.Dir(*dbPath), 0o755); err != nil {
		log.Fatalf("create db directory: %v", err)
	}
	st, err := store.OpenStore(*dbPath)
	if err != nil {
		log.Fatalf("open store: %v", err)
	}
	defer st.Close()

	go func() {
		feed.RefreshAll(st)
		for range time.Tick(*refresh) {
			feed.RefreshAll(st)
		}
	}()

	mux := http.NewServeMux()
	mux.HandleFunc("GET /{$}", shellHandler(st))
	mux.HandleFunc("GET /fragments/feeds", feedListFragmentHandler(st))
	mux.HandleFunc("GET /fragments/articles", articleListFragmentHandler(st))
	mux.HandleFunc("GET /fragments/article/{id}", articleFragmentHandler(st))
	mux.HandleFunc("GET /fragments/search", searchFragmentHandler(st))
	mux.HandleFunc("POST /api/feeds", addFeedHandler(st))
	mux.HandleFunc("DELETE /api/feeds/{id}", feedByIDHandler(st))
	mux.HandleFunc("POST /api/feeds/{id}/refresh", feedRefreshHandler(st))
	mux.HandleFunc("POST /api/refresh", refreshAllHandler(st))
	mux.HandleFunc("POST /api/articles/read-all", markAllReadHandler(st))
	mux.HandleFunc("POST /api/articles/clear", clearArticlesHandler(st))
	mux.HandleFunc("GET /api/stats", statsHandler(st))
	mux.HandleFunc("GET /api/settings", settingsHandler(st))
	mux.HandleFunc("POST /api/settings", settingsHandler(st))
	mux.HandleFunc("POST /api/opml/import", opmlImportHandler(st))
	mux.HandleFunc("POST /api/opml/export", opmlExportHandler(st, *dbPath))
	mux.Handle("GET /", staticHandler())

	// loopback only, backs the window below
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		log.Fatalf("listen: %v", err)
	}
	go http.Serve(ln, mux)

	w := webview.New(false)
	defer w.Destroy()
	w.SetTitle("OpenRSS")
	w.SetSize(1100, 800, webview.HintNone)
	w.Navigate("http://" + ln.Addr().String())

	w.Bind("openExternal", func(rawURL string) error {
		u, err := url.Parse(rawURL)
		if err != nil || (u.Scheme != "http" && u.Scheme != "https") {
			return fmt.Errorf("refusing to open non-http(s) url")
		}
		switch runtime.GOOS {
		case "darwin":
			return exec.Command("open", rawURL).Start()
		case "linux":
			return exec.Command("xdg-open", rawURL).Start()
		}
		return fmt.Errorf("unsupported platform: %s", runtime.GOOS)
	})

	// macOS OPML import
	if runtime.GOOS == "darwin" {
		w.Bind("nativeImportOPML", func() (map[string]any, error) {
			out, err := exec.Command("osascript", "-e",
				`POSIX path of (choose file with prompt "Select an OPML file")`).Output()
			if err != nil {
				return map[string]any{"canceled": true}, nil
			}
			data, err := os.ReadFile(strings.TrimSpace(string(out)))
			if err != nil {
				return nil, err
			}
			imported, err := ImportOPMLData(st, data)
			if err != nil {
				return nil, err
			}
			return map[string]any{"imported": imported}, nil
		})
	}

	w.Run()
}
