package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"path/filepath"

	"github.com/imtiyazkumar/yember-core/config"
	"github.com/imtiyazkumar/yember-core/migrations"
	"github.com/imtiyazkumar/yember-core/pkg/db"
	"github.com/joho/godotenv"
)

func main() {
	selected := flag.String("file", "", "apply only this historical migration identity from manifest.json")
	baseline := flag.String("baseline-through", "", "record existing migrations through this historical identity without executing them; inspect the schema first")
	demo := flag.Bool("include-demo-seed", false, "include demo catalog and users; never enable on production")
	flag.Parse()
	if *selected != "" {
		*selected = filepath.Base(*selected)
	}
	_ = godotenv.Load()
	cfg, err := config.Load()
	if err != nil {
		log.Fatal(err)
	}
	plan, err := migrations.Plan()
	if err != nil {
		log.Fatal(err)
	}
	pool, err := db.NewPool(cfg)
	if err != nil {
		log.Fatal(err)
	}
	defer pool.Close()
	err = migrations.Apply(context.Background(), pool, plan, migrations.Options{
		File: *selected, BaselineThrough: *baseline, IncludeDemo: *demo,
		Applied: func(name string) { fmt.Printf("Applied %s\n", name) },
	})
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println("All migrations applied successfully")
}
