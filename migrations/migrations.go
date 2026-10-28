// Package migrations keeps SQL organized by domain while preserving historical
// transaction boundaries and migration identities for existing installations.
package migrations

import (
	"context"
	"crypto/sha256"
	"embed"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io/fs"
	"path"
	"regexp"
	"sort"
	"strings"

	"github.com/jackc/pgx/v5/pgxpool"
)

//go:embed legacy/*.sql legacy/manifest.json
var legacyFiles embed.FS

var files = func() fs.FS {
	source, err := fs.Sub(legacyFiles, "legacy")
	if err != nil {
		panic(err)
	}
	return source
}()

type Migration struct {
	Filename        string   `json:"filename"`
	Checksum        string   `json:"checksum"`
	LegacyChecksums []string `json:"legacy_checksums,omitempty"`
	Demo            bool     `json:"demo"`
	Steps           []string `json:"steps"`
	SQL             string   `json:"-"`
}
type manifest struct {
	Version    int         `json:"version"`
	Domains    []string    `json:"domains"`
	Migrations []Migration `json:"migrations"`
}

func digest(data []byte) string { sum := sha256.Sum256(data); return hex.EncodeToString(sum[:]) }

// Plan validates every SQL block before returning the historical execution order.
func Plan() ([]Migration, error) { return Load(files) }

var marker = regexp.MustCompile(`(?m)^-- yember-step: ([a-zA-Z0-9_./-]+)\n`)

// Load accepts a filesystem so corruption and packaging can be tested without a DB.
func Load(source fs.FS) ([]Migration, error) {
	data, err := fs.ReadFile(source, "manifest.json")
	if err != nil {
		return nil, err
	}
	var m manifest
	if err = json.Unmarshal(data, &m); err != nil {
		return nil, err
	}
	if m.Version != 1 {
		return nil, fmt.Errorf("unsupported migration manifest version %d", m.Version)
	}
	blocks := map[string]string{}
	domainNames := map[string]bool{}
	for _, name := range m.Domains {
		if path.Base(name) != name || domainNames[name] {
			return nil, fmt.Errorf("invalid or duplicate domain %q", name)
		}
		domainNames[name] = true
		data, err = fs.ReadFile(source, name)
		if err != nil {
			return nil, err
		}
		matches := marker.FindAllSubmatchIndex(data, -1)
		if len(matches) == 0 {
			return nil, fmt.Errorf("domain %s has no SQL blocks", name)
		}
		for _, line := range strings.Split(string(data[:matches[0][0]]), "\n") {
			line = strings.TrimSpace(line)
			if line != "" && !strings.HasPrefix(line, "--") {
				return nil, fmt.Errorf("unmarked SQL in domain %s", name)
			}
		}
		for i, match := range matches {
			end := len(data)
			if i+1 < len(matches) {
				end = matches[i+1][0]
			}
			body := string(data[match[1]:end])
			const suffix = "\n-- yember-end-step\n"
			if !strings.HasSuffix(body, suffix) {
				return nil, fmt.Errorf("invalid SQL block in %s", name)
			}
			id := string(data[match[2]:match[3]])
			if _, exists := blocks[id]; exists {
				return nil, fmt.Errorf("duplicate SQL block %s", id)
			}
			blocks[id] = strings.TrimSuffix(body, suffix)
		}
	}
	names := map[string]bool{}
	for i := range m.Migrations {
		entry := &m.Migrations[i]
		if path.Base(entry.Filename) != entry.Filename || names[entry.Filename] {
			return nil, fmt.Errorf("invalid or duplicate migration %q", entry.Filename)
		}
		names[entry.Filename] = true
		var sql strings.Builder
		for _, id := range entry.Steps {
			block, ok := blocks[id]
			if !ok {
				return nil, fmt.Errorf("missing or reused SQL block %s", id)
			}
			sql.WriteString(block)
			delete(blocks, id)
		}
		entry.SQL = sql.String()
		if digest([]byte(entry.SQL)) != entry.Checksum {
			return nil, fmt.Errorf("migration checksum mismatch: %s", entry.Filename)
		}
	}
	if len(blocks) != 0 {
		return nil, fmt.Errorf("%d unreferenced SQL blocks", len(blocks))
	}
	// New standalone migrations remain supported without editing historical SQL.
	extra, err := fs.Glob(source, "*.sql")
	if err != nil {
		return nil, err
	}
	for _, name := range extra {
		if domainNames[name] || names[name] {
			continue
		}
		data, err = fs.ReadFile(source, name)
		if err != nil {
			return nil, err
		}
		m.Migrations = append(m.Migrations, Migration{Filename: name, Checksum: digest(data), SQL: string(data)})
	}
	sort.Slice(m.Migrations, func(i, j int) bool { return m.Migrations[i].Filename < m.Migrations[j].Filename })
	return m.Migrations, nil
}

type Options struct {
	File            string
	BaselineThrough string
	IncludeDemo     bool
	Applied         func(string)
}

// Apply never rewrites history: old identities, checksums and timestamps remain.
func Apply(ctx context.Context, pool *pgxpool.Pool, plan []Migration, opts Options) error {
	known := map[string]bool{}
	for _, m := range plan {
		known[m.Filename] = true
	}
	if opts.File != "" && !known[opts.File] {
		return fmt.Errorf("unknown migration identity %q; domain files require the complete migration plan", opts.File)
	}
	if opts.BaselineThrough != "" && !known[opts.BaselineThrough] {
		return fmt.Errorf("unknown baseline migration %q", opts.BaselineThrough)
	}
	var existing bool
	if err := pool.QueryRow(ctx, `SELECT to_regclass('public.users') IS NOT NULL`).Scan(&existing); err != nil {
		return err
	}
	if _, err := pool.Exec(ctx, `CREATE TABLE IF NOT EXISTS public.schema_migrations(filename TEXT PRIMARY KEY,checksum TEXT NOT NULL,applied_at TIMESTAMPTZ NOT NULL DEFAULT now())`); err != nil {
		return err
	}
	var tracked int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM public.schema_migrations`).Scan(&tracked); err != nil {
		return err
	}
	if existing && tracked == 0 && opts.BaselineThrough == "" && opts.File == "" {
		return fmt.Errorf("existing untracked database: inspect schema and use --baseline-through <last-applied-file>; historical migrations will not be replayed")
	}
	for _, m := range plan {
		if (m.Demo && !opts.IncludeDemo) || (opts.File != "" && m.Filename != opts.File) {
			continue
		}
		if err := applyOne(ctx, pool, m, opts); err != nil {
			return fmt.Errorf("migration %s: %w", m.Filename, err)
		}
	}
	return nil
}

func applyOne(ctx context.Context, pool *pgxpool.Pool, m Migration, opts Options) error {
	tx, err := pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	if _, err = tx.Exec(ctx, `SELECT pg_advisory_xact_lock(894212)`); err != nil {
		return err
	}
	var previous string
	if err = tx.QueryRow(ctx, `SELECT COALESCE((SELECT checksum FROM public.schema_migrations WHERE filename=$1),'')`, m.Filename).Scan(&previous); err != nil {
		return err
	}
	if previous != "" {
		if previous == m.Checksum {
			return nil
		}
		for _, checksum := range m.LegacyChecksums {
			if previous == checksum {
				return nil
			}
		}
		return fmt.Errorf("recorded checksum mismatch")
	}
	if opts.BaselineThrough == "" || m.Filename > opts.BaselineThrough {
		if _, err = tx.Exec(ctx, m.SQL); err != nil {
			return err
		}
	}
	if _, err = tx.Exec(ctx, `INSERT INTO public.schema_migrations(filename,checksum) VALUES($1,$2)`, m.Filename, m.Checksum); err != nil {
		return err
	}
	if err = tx.Commit(ctx); err != nil {
		return err
	}
	if opts.Applied != nil {
		opts.Applied(m.Filename)
	}
	return nil
}
