package main

import (
	"flag"
	"log"
	"net/http"
	"os"
	"time"

	"github.com/go-chi/chi/v5"
	chimw "github.com/go-chi/chi/v5/middleware"
	"github.com/oktaaokta/hostly/internal/domain"
	"github.com/oktaaokta/hostly/internal/handler"
	"github.com/oktaaokta/hostly/internal/repository"
	"github.com/oktaaokta/hostly/internal/usecase"
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

	addr := ":" + port
	server := http.Handler(r)
	if basePath != "" {
		server = http.StripPrefix(basePath, r)
	}
	log.Printf("hostly listening on %s%s", addr, basePath)
	log.Fatal(http.ListenAndServe(addr, server))
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
