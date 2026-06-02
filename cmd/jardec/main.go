package main

import (
	"context"
	"fmt"
	"log"
	"os"

	appcli "jardec/internal/cli"
	"jardec/internal/patch"
	"jardec/internal/pipeline"
	ireport "jardec/internal/report"
	"jardec/internal/sourcepatch"
)

func main() {
	engine := pipeline.Engine{}
	patchEngine := patch.Engine{}
	sourcePatchEngine := sourcepatch.Engine{}
	app := appcli.NewApp(func(ctx context.Context, cfg appcli.Config) error {
		rep, err := engine.Run(ctx, pipeline.Config{
			InputPath:        cfg.InputPath,
			OutputDir:        cfg.OutputDir,
			JadxPath:         cfg.JadxPath,
			ProcyonPath:   cfg.ProcyonPath,
			ExtraClasspath:   cfg.ExtraClasspath,
			TempDir:          cfg.TempDir,
			KeepTemp:         cfg.KeepTemp,
			RetryConcurrency: cfg.RetryConcurrency,
		})
		if err != nil {
			return err
		}

		fmt.Print(ireport.RenderText(rep))
		return nil
	}, func(ctx context.Context, cfg appcli.PatchConfig) error {
		rep, err := patchEngine.Run(ctx, cfg)
		if err != nil {
			return err
		}

		fmt.Print(ireport.RenderPatchText(rep))
		return nil
	}, func(ctx context.Context, cfg appcli.SourcePatchConfig) error {
		rep, err := sourcePatchEngine.Run(ctx, cfg)
		if rep.InputJar != "" || rep.OutputJar != "" {
			fmt.Print(ireport.RenderPatchText(rep))
		}
		return err
	}, nil)
	if err := app.Run(os.Args); err != nil {
		log.Fatal(err)
	}
}
