package main

import (
	"context"
	"flag"
	"github.com/imtiyazkumar/yember-core/internal/contractschema"
	"github.com/imtiyazkumar/yember-core/internal/devseed"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/joho/godotenv"
	"log"
	"os"
)

func main() {
	allow := flag.Bool("allow-local-yember", false, "allow the inspected loopback development yember database")
	flag.Parse()
	_ = godotenv.Load()
	dsn := os.Getenv("POSTGRES_DSN")
	if err := devseed.ValidateTarget(os.Getenv("APP_ENV"), dsn); err != nil {
		cfg, e := pgxpool.ParseConfig(dsn)
		if !*allow || e != nil || os.Getenv("APP_ENV") != "development" || cfg.ConnConfig.Database != "yember" || (cfg.ConnConfig.Host != "localhost" && cfg.ConnConfig.Host != "127.0.0.1") {
			log.Fatal(err)
		}
	}
	p, err := pgxpool.New(context.Background(), dsn)
	if err != nil {
		log.Fatal("database configuration failed")
	}
	defer p.Close()
	if err = contractschema.Validate(context.Background(), p); err != nil {
		log.Fatal(err)
	}
	if err = devseed.SeedDocument(context.Background(), p, os.Getenv("LOCAL_SEED_PASSWORD")); err != nil {
		log.Fatal(err)
	}
	log.Print("Document fixtures ready. Existing users and passwords preserved. Registration fees remain DUE; no payment, order or payout history was fabricated.")
}
