package cli

import (
	"context"
	"fmt"
	"os/exec"
	"runtime"

	urfavecli "github.com/urfave/cli/v2"
)

type RunFunc func(context.Context, Config) error

type PatchRunFunc func(context.Context, PatchConfig) error

type LookupFunc func(string) (string, error)

func NewApp(run RunFunc, patchRun PatchRunFunc, lookup LookupFunc) *urfavecli.App {
	return newAppWithDeps(run, patchRun, lookup, loadProjectConfigFromWorkingDir)
}

func newAppWithDeps(run RunFunc, patchRun PatchRunFunc, lookup LookupFunc, loadProjectConfig func() (ProjectConfig, error)) *urfavecli.App {
	if run == nil {
		run = func(context.Context, Config) error { return nil }
	}
	if patchRun == nil {
		patchRun = func(context.Context, PatchConfig) error { return nil }
	}
	if lookup == nil {
		lookup = exec.LookPath
	}
	if loadProjectConfig == nil {
		loadProjectConfig = func() (ProjectConfig, error) { return ProjectConfig{}, nil }
	}

	return &urfavecli.App{
		Name:  "jardec",
		Usage: "Decompile JARs with jadx-first and cfr-fallback recovery",
		Flags: []urfavecli.Flag{
			&urfavecli.StringFlag{
				Name:    "input",
				Aliases: []string{"i"},
				Usage:   "Path to the input JAR file",
			},
			&urfavecli.StringFlag{
				Name:    "output",
				Aliases: []string{"o"},
				Usage:   "Directory for final decompiled output",
			},
			&urfavecli.StringFlag{
				Name:  "jadx-path",
				Usage: "Path to the jadx executable",
			},
			&urfavecli.StringFlag{
				Name:  "cfr-path",
				Usage: "Path to the cfr executable, wrapper script, or jar file",
			},
			&urfavecli.StringFlag{
				Name:  "temp-dir",
				Usage: "Base directory for temporary workspaces",
			},
			&urfavecli.BoolFlag{
				Name:  "keep-temp",
				Usage: "Preserve temporary workspaces after execution",
			},
			&urfavecli.IntFlag{
				Name:        "retry-concurrency",
				Usage:       "Maximum concurrent cfr retry workers",
				DefaultText: fmt.Sprintf("%d (CPU count)", runtime.NumCPU()),
			},
		},
		Action: func(ctx *urfavecli.Context) error {
			cfg, err := ConfigFromContext(ctx)
			if err != nil {
				return err
			}
			projectConfig, err := loadProjectConfig()
			if err != nil {
				return err
			}
			cfg = ApplyProjectConfig(cfg, projectConfig)
			cfg, err = ValidateConfig(cfg, lookup)
			if err != nil {
				return err
			}
			return run(ctx.Context, cfg)
		},
		Commands: []*urfavecli.Command{
			{
				Name:  "patch-classes",
				Usage: "Patch a JAR with compiled class outputs",
				Flags: []urfavecli.Flag{
					&urfavecli.StringFlag{
						Name:     "input-jar",
						Usage:    "Path to the original input JAR file",
						Required: true,
					},
					&urfavecli.StringFlag{
						Name:     "classes-dir",
						Usage:    "Directory containing compiled replacement class files",
						Required: true,
					},
					&urfavecli.StringFlag{
						Name:     "output-jar",
						Usage:    "Path to the patched output JAR file",
						Required: true,
					},
					&urfavecli.BoolFlag{
						Name:  "dry-run",
						Usage: "Preview patch changes and write reports without creating the patched JAR",
					},
					&urfavecli.StringSliceFlag{
						Name:  "class",
						Usage: "Restrict patching to explicit top-level binary class names (repeatable)",
					},
				},
				Action: func(ctx *urfavecli.Context) error {
					cfg, err := PatchConfigFromContext(ctx)
					if err != nil {
						return err
					}
					cfg, err = ValidatePatchConfig(cfg)
					if err != nil {
						return err
					}
					return patchRun(ctx.Context, cfg)
				},
			},
		},
	}
}
