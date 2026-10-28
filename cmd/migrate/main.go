package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"log"
	"os"

	"github.com/imtiyazkumar/yember-core/config"
	"github.com/imtiyazkumar/yember-core/internal/contractschema"
	"github.com/imtiyazkumar/yember-core/pkg/db"
	"github.com/joho/godotenv"
)

func main() {
	replace := flag.Bool("replace-empty-legacy", false, "replace a backed-up legacy installation only if all operational tables are empty")
	backup := flag.String("backup-file", "", "verified pg_dump custom-format backup required for legacy replacement")
	flag.Parse()
	if flag.NArg() != 0 {
		log.Fatal("unexpected arguments")
	}
	_ = godotenv.Load()
	cfg, err := config.Load()
	if err != nil {
		log.Fatal(err)
	}
	pool, err := db.NewPool(cfg)
	if err != nil {
		log.Fatal("database connection configuration failed")
	}
	defer pool.Close()
	ctx := context.Background()
	if *replace {
		if *backup == "" {
			log.Fatal("--replace-empty-legacy requires --backup-file; existing records are never reset")
		}
		file, err := os.Open(*backup)
		if err != nil {
			log.Fatal("backup file cannot be read")
		}
		header := make([]byte, 5)
		_, readErr := io.ReadFull(file, header)
		info, statErr := file.Stat()
		file.Close()
		if readErr != nil || statErr != nil || string(header) != "PGDMP" || info.Size() < 100 {
			log.Fatal("backup must be a verified pg_dump custom-format archive")
		}
		err = contractschema.ReplaceEmptyLegacy(ctx, pool)
	} else {
		err = contractschema.Install(ctx, pool)
	}
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println("Document schema verified: exactly 38 application tables, bigint internal relationships and public UUIDs. No legacy tables.")
}
