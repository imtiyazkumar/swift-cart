package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"net"
	"os"
	"strings"
	"time"

	"github.com/imtiyazkumar/yember-core/internal/contractschema"
	"github.com/imtiyazkumar/yember-core/internal/demoseed"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/joho/godotenv"
)

func main() {
	customers := flag.Int("customers", 600, "number of synthetic customers")
	restaurants := flag.Int("restaurants", 60, "number of synthetic restaurants")
	riders := flag.Int("riders", 120, "number of synthetic delivery agents")
	orders := flag.Int("orders", 5000, "number of synthetic orders")
	dry := flag.Bool("dry-run", false, "execute and validate fixtures, then roll back")
	verify := flag.Bool("verify", false, "verify committed demo data and sample documents without changes")
	inspect := flag.Bool("inspect", false, "print current table counts without changing data")
	allow := flag.Bool("allow-local-yember", false, "permit the configured loopback development yember database")
	flag.Parse()
	_ = godotenv.Load()
	cfg, err := pgxpool.ParseConfig(os.Getenv("POSTGRES_DSN"))
	if err != nil {
		log.Fatal("invalid database configuration")
	}
	local := strings.EqualFold(cfg.ConnConfig.Host, "localhost")
	if ip := net.ParseIP(cfg.ConnConfig.Host); ip != nil {
		local = ip.IsLoopback()
	}
	if !local || (os.Getenv("APP_ENV") != "development" && os.Getenv("APP_ENV") != "test") || (cfg.ConnConfig.Database != "yember_local" && cfg.ConnConfig.Database != "yember_local_test" && !(*allow && cfg.ConnConfig.Database == "yember" && os.Getenv("APP_ENV") == "development")) {
		log.Fatal("demo seeding is restricted to explicitly allowed loopback development databases")
	}
	cfg.ConnConfig.ConnectTimeout = 5 * time.Second
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Minute)
	defer cancel()
	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		log.Fatal("could not configure development database")
	}
	defer pool.Close()
	if err = contractschema.Validate(ctx, pool); err != nil {
		log.Fatal(err)
	}
	fmt.Printf("Development target: %s / %s\n", cfg.ConnConfig.Host, cfg.ConnConfig.Database)
	if *verify {
		result, e := demoseed.VerifyDemo(ctx, pool, os.Getenv("DOCUMENT_LOCAL_DIR"))
		if e != nil {
			log.Fatal(e)
		}
		encoded, _ := json.MarshalIndent(result, "", "  ")
		fmt.Println(string(encoded))
		return
	}
	var counts []demoseed.DemoCount
	if *inspect {
		counts, err = demoseed.DemoCounts(ctx, pool)
	} else {
		root := os.Getenv("DOCUMENT_LOCAL_DIR")
		counts, err = demoseed.SeedDemo(ctx, pool, demoseed.DemoOptions{Customers: *customers, Restaurants: *restaurants, Riders: *riders, Orders: *orders, Password: os.Getenv("LOCAL_SEED_PASSWORD"), DocumentDirectory: root, DryRun: *dry, Progress: func(s string) { log.Print(s) }})
	}
	if err != nil {
		log.Fatal(err)
	}
	var added int64
	for _, count := range counts {
		added += count.After - count.Before
	}
	encoded, _ := json.MarshalIndent(counts, "", "  ")
	fmt.Println(string(encoded))
	fmt.Printf("Tables: %d; rows added: %d; dry run: %t\n", len(counts), added, *dry)
}
