package main

import (
	"context"
	"fmt"
	"log"
	"os"

	appcli "jardec/internal/cli"
	"jardec/internal/pipeline"
	ireport "jardec/internal/report"
)

func main() {
	engine := pipeline.Engine{}
	app := appcli.NewApp(func(ctx context.Context, cfg appcli.Config) error {
		rep, err := engine.Run(ctx, pipeline.Config{
			InputPath:        cfg.InputPath,
			OutputDir:        cfg.OutputDir,
			JadxPath:         cfg.JadxPath,
			CfrPath:          cfg.CfrPath,
			TempDir:          cfg.TempDir,
			KeepTemp:         cfg.KeepTemp,
			RetryConcurrency: cfg.RetryConcurrency,
		})
		if err != nil {
			return err
		}

		fmt.Print(ireport.RenderText(rep))
		return nil
	}, nil)
	if err := app.Run(os.Args); err != nil {
		log.Fatal(err)
	}
}
