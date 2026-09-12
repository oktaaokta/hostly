package main

import (
	"flag"
	"log"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	chimw "github.com/go-chi/chi/v5/middleware"
	"github.com/oktaaokta/hostly/internal/domain"
	"github.com/oktaaokta/hostly/internal/handler"
	"github.com/oktaaokta/hostly/internal/repository"
	"github.com/oktaaokta/hostly/internal/usecase"
	"github.com/oktaaokta/hostly/internal/webassets"
)

func main() {
	seed := flag.Bool("seed", false, "create the demo venue if missing")
	flag.Parse()

	port := envOr("PORT", "8080")
	dbPath := envOr("DB_PATH", "hostly.db")
	basePath := envOr("BASE_PATH", "")

	db, err := repository.OpenSQLite(dbPath)
	if err != nil {
		log.Fatalf("open db: %v", err)
	}
	defer db.Close()

	q := usecase.NewQueue(db.Venues(), db.Parties(), time.Now)

	if *seed {
		seedVenue(db.Venues())
	}

	h := handler.New(q, handler.NewHub())
	r := chi.NewRouter()
	r.Use(chimw.Logger)
	r.Use(chimw.Recoverer)
	h.RegisterRoutes(r)
	r.NotFound(serveSPA)

	addr := ":" + port
	server := http.Handler(r)
	if basePath != "" {
		server = http.StripPrefix(basePath, r)
	}
	log.Printf("hostly listening on %s%s", addr, basePath)
	log.Fatal(http.ListenAndServe(addr, server))
}

// serveSPA serves the embedded React app. Real files are served as-is; every
// other path returns index.html so client-side routes (/q/..., /staff/...)
// work on refresh and direct links.
func serveSPA(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/")
	if path != "" && !strings.HasPrefix(path, "assets/") && !strings.HasSuffix(path, ".svg") && !strings.Contains(path, ".") {
		path = "index.html"
	}
	if path == "" {
		path = "index.html"
	}
	b, err := webassets.Dist.ReadFile("dist/" + path)
	if err != nil {
		b, err = webassets.Dist.ReadFile("dist/index.html")
		if err != nil {
			http.Error(w, "not found", http.StatusNotFound)
			return
		}
	}
	w.Header().Set("Content-Type", mimeTypeByPath(path))
	w.Write(b)
}

func mimeTypeByPath(path string) string {
	switch {
	case strings.HasSuffix(path, ".js"):
		return "text/javascript"
	case strings.HasSuffix(path, ".css"):
		return "text/css"
	case strings.HasSuffix(path, ".svg"):
		return "image/svg+xml"
	case strings.HasSuffix(path, ".png"):
		return "image/png"
	default:
		return "text/html; charset=utf-8"
	}
}

func seedVenue(vr domain.VenueRepository) {
	v := &domain.Venue{Slug: "joes-diner", Name: "Joe's Diner", OpenTime: "10:00", CloseTime: "22:00", StaffToken: "demo-staff-token"}
	if _, err := vr.GetBySlug(v.Slug); err == nil {
		log.Println("seed: joes-diner already exists")
		return
	}
	if err := vr.Create(v); err != nil {
		log.Fatalf("seed: %v", err)
	}
	log.Println("seed: joes-diner created — staff URL /staff/joes-diner?token=demo-staff-token")
}

func envOr(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}
