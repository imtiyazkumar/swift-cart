package main

import (
	"context"
	"github.com/imtiyazkumar/yember-core/internal/contractschema"
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
		log.Fatal("database configuration failed")
	}
	defer p.Close()
	if err = contractschema.Validate(context.Background(), p); err != nil {
		log.Fatal(err)
	}
	if err = devseed.SeedDocumentStations(context.Background(), p); err != nil {
		log.Fatal(err)
	}
	log.Print("Document service station ready. Existing records preserved.")
}
