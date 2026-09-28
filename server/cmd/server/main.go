package main

import (
	"log"
	"net/http"
	"os"

	"ripple/server/internal/api"
	"ripple/server/internal/auth"
	"ripple/server/internal/config"
	"ripple/server/internal/db"
	"ripple/server/internal/store"
	"ripple/server/internal/ws"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		log.Fatal(err)
	}
	if err := os.MkdirAll(cfg.UploadDir, 0o755); err != nil {
		log.Fatal(err)
	}
	sqlDB, err := db.Open(cfg.MySQLDSN)
	if err != nil {
		log.Fatal(err)
	}
	defer sqlDB.Close()
	if err := db.Migrate(sqlDB); err != nil {
		log.Fatal(err)
	}
	if err := db.SeedDemo(sqlDB); err != nil {
		log.Fatal(err)
	}

	authSvc := &auth.Service{DB: sqlDB, Secret: []byte(cfg.JWTSecret)}
	st := store.New(sqlDB, cfg.MaxRounds)
	hub := ws.NewHub(authSvc, st)
	handler := api.New(cfg, authSvc, st, hub)

	log.Printf("ripple listening on %s (mysql dsn configured, uploads=%s)", cfg.Addr, cfg.UploadDir)
	if err := http.ListenAndServe(cfg.Addr, handler); err != nil {
		log.Fatal(err)
	}
}
