package main

import (
	"flag"
	"log"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	webview "github.com/webview/webview_go"
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
	// wayland stuff
	if runtime.GOOS == "linux" && os.Getenv("GDK_BACKEND") == "" {
		os.Setenv("GDK_BACKEND", "x11")
	}
	// gpu linux stuff
	if runtime.GOOS == "linux" && os.Getenv("WEBKIT_DISABLE_DMABUF_RENDERER") == "" {
		os.Setenv("WEBKIT_DISABLE_DMABUF_RENDERER", "1")
	}

	dbPath := flag.String("db", defaultDBPath(), "sqlite database path")
	opmlPath := flag.String("opml", "", "OPML file to import feeds from on startup")
	refresh := flag.Duration("refresh", 30*time.Minute, "feed refresh interval")
	flag.Parse()

	if err := os.MkdirAll(filepath.Dir(*dbPath), 0o755); err != nil {
		log.Fatalf("create db directory: %v", err)
	}
	store, err := OpenStore(*dbPath)
	if err != nil {
		log.Fatalf("open store: %v", err)
	}
	defer store.Close()

	imageCacheDir = filepath.Join(filepath.Dir(*dbPath), "imagecache")
	if err := os.MkdirAll(imageCacheDir, 0o755); err != nil {
		log.Printf("create image cache dir: %v (image caching disabled)", err)
		imageCacheDir = ""
	}

	if *opmlPath != "" {
		feeds, err := LoadOPML(*opmlPath)
		if err != nil {
			log.Fatalf("load opml: %v", err)
		}
		for _, f := range feeds {
			if _, err := store.AddFeed(f.URL, f.Title, f.SiteURL); err != nil {
				log.Printf("add feed %s: %v", f.URL, err)
			}
		}
		log.Printf("imported %d feeds from %s", len(feeds), *opmlPath)
	}

	go func() {
		RefreshAll(store)
		for range time.Tick(*refresh) {
			RefreshAll(store)
		}
	}()

	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/feeds", feedsHandler(store))
	mux.HandleFunc("POST /api/feeds", feedsHandler(store))
	mux.HandleFunc("DELETE /api/feeds/{id}", feedByIDHandler(store))
	mux.HandleFunc("POST /api/feeds/{id}/refresh", feedRefreshHandler(store))
	mux.HandleFunc("POST /api/refresh", refreshAllHandler(store))
	mux.HandleFunc("GET /api/articles", articlesHandler(store))
	mux.HandleFunc("GET /api/articles/{id}", articleByIDHandler(store))
	mux.HandleFunc("POST /api/articles/read-all", markAllReadHandler(store))
	mux.HandleFunc("POST /api/articles/clear", clearArticlesHandler(store))
	mux.HandleFunc("GET /api/search", searchHandler(store))
	mux.HandleFunc("GET /api/stats", statsHandler(store))
	mux.HandleFunc("POST /api/opml/import", opmlImportHandler(store))
	mux.HandleFunc("POST /api/opml/export", opmlExportHandler(store, *dbPath))
	mux.HandleFunc("GET /api/images/{hash}", imagesHandler())
	mux.Handle("/", staticHandler())

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

	// macOS native file dialog for OPML import
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
			imported, err := ImportOPMLData(store, data)
			if err != nil {
				return nil, err
			}
			return map[string]any{"imported": imported}, nil
		})
	}

	w.Run()
}
