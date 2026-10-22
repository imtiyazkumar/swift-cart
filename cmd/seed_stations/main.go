package main

import (
	"context"
	"github.com/imtiyazkumar/yember-core/internal/devseed"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/joho/godotenv"
	"log"
	"os"
)

func main() {
	_ = godotenv.Load()
	dsn := os.Getenv("POSTGRES_DSN")
	if err := devseed.ValidateTarget(os.Getenv("APP_ENV"), dsn); err != nil {
		log.Fatal(err)
	}
	p, err := pgxpool.New(context.Background(), dsn)
	if err != nil {
		log.Fatal("could not open local database")
	}
	defer p.Close()
	if err = devseed.SeedStations(context.Background(), p); err != nil {
		log.Fatal(err)
	}
	log.Print("Three local stations ready; known demo station relations repaired. Accounts preserved.")
}
