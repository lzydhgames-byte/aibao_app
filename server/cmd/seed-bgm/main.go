// Command seed-bgm reads a YAML manifest and upserts rows into bgm_assets.
//
// Usage:
//
//	go run ./cmd/seed-bgm -manifest=safety/bgm_manifest.yaml -config=config/config.dev.yaml
package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"os"

	"gopkg.in/yaml.v3"

	"github.com/aibao/server/internal/model"
	"github.com/aibao/server/internal/pkg/config"
	"github.com/aibao/server/internal/repository"
)

type manifestEntry struct {
	Mood        string `yaml:"mood"`
	Filename    string `yaml:"filename"`
	ObjectKey   string `yaml:"object_key"`
	DurationSec int    `yaml:"duration_sec"`
	License     string `yaml:"license"`
}

type manifest struct {
	BGM []manifestEntry `yaml:"bgm"`
}

func main() {
	manifestPath := flag.String("manifest", "safety/bgm_manifest.yaml", "path to BGM manifest YAML")
	configPath := flag.String("config", "config/config.dev.yaml", "path to server config YAML")
	flag.Parse()

	cfg, err := config.Load(*configPath)
	must(err)

	raw, err := os.ReadFile(*manifestPath)
	must(err)

	var m manifest
	must(yaml.Unmarshal(raw, &m))
	if len(m.BGM) == 0 {
		slog.Error("seed.bgm.empty_manifest", "path", *manifestPath)
		os.Exit(2)
	}

	db, err := repository.NewDB(cfg.Postgres)
	must(err)
	defer repository.Close(db)

	repo := repository.NewBGMRepo(db)
	ctx := context.Background()

	for _, b := range m.BGM {
		err := repo.Upsert(ctx, &model.BGMAsset{
			Mood:        b.Mood,
			Filename:    b.Filename,
			ObjectKey:   b.ObjectKey,
			DurationSec: b.DurationSec,
			License:     b.License,
			Active:      true,
		})
		if err != nil {
			slog.Error("seed.bgm.upsert.fail", "filename", b.Filename, "err", err.Error())
			os.Exit(2)
		}
		fmt.Printf("upserted %s (mood=%s)\n", b.Filename, b.Mood)
	}
	fmt.Printf("seeded %d BGM assets\n", len(m.BGM))
}

func must(err error) {
	if err != nil {
		slog.Error("seed.bgm.fatal", "err", err.Error())
		os.Exit(1)
	}
}
