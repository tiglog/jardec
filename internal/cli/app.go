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
type SourcePatchRunFunc func(context.Context, SourcePatchConfig) error

type LookupFunc func(string) (string, error)

func resolveProjectConfig(ctx *urfavecli.Context, defaultLoader func() (ProjectConfig, error)) (ProjectConfig, error) {
	if configPath := ctx.String("config"); configPath != "" {
		return LoadProjectConfigFromPath(configPath)
	}
	return defaultLoader()
}

func NewApp(run RunFunc, patchRun PatchRunFunc, sourcePatchRun SourcePatchRunFunc, lookup LookupFunc) *urfavecli.App {
	return newAppWithAllDeps(run, patchRun, sourcePatchRun, lookup, loadProjectConfigFromWorkingDir)
}

func newAppWithDeps(run RunFunc, patchRun PatchRunFunc, lookup LookupFunc, loadProjectConfig func() (ProjectConfig, error)) *urfavecli.App {
	return newAppWithAllDeps(run, patchRun, nil, lookup, loadProjectConfig)
}

func newSourcePatchAppWithDeps(run RunFunc, patchRun PatchRunFunc, sourcePatchRun SourcePatchRunFunc, lookup LookupFunc, loadProjectConfig func() (ProjectConfig, error)) *urfavecli.App {
	return newAppWithAllDeps(run, patchRun, sourcePatchRun, lookup, loadProjectConfig)
}

func newAppWithAllDeps(run RunFunc, patchRun PatchRunFunc, sourcePatchRun SourcePatchRunFunc, lookup LookupFunc, loadProjectConfig func() (ProjectConfig, error)) *urfavecli.App {
	if run == nil {
		run = func(context.Context, Config) error { return nil }
	}
	if patchRun == nil {
		patchRun = func(context.Context, PatchConfig) error { return nil }
	}
	if sourcePatchRun == nil {
		sourcePatchRun = func(context.Context, SourcePatchConfig) error { return nil }
	}
	if lookup == nil {
		lookup = exec.LookPath
	}
	if loadProjectConfig == nil {
		loadProjectConfig = func() (ProjectConfig, error) { return ProjectConfig{}, nil }
	}

	return &urfavecli.App{
		Name: "jardec",
		Flags: []urfavecli.Flag{
			&urfavecli.StringFlag{
				Name:  "config",
				Usage: "Path to config.yaml (default: search upward from current working directory)",
			},
		},
		Action: func(ctx *urfavecli.Context) error {
			if err := urfavecli.ShowAppHelp(ctx); err != nil {
				return err
			}
			return fmt.Errorf("a subcommand is required")
		},
		Commands: []*urfavecli.Command{
			{
				Name:  "decompile",
				Usage: "Decompile a JAR with jadx-first and cfr-fallback recovery",
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
					&urfavecli.StringSliceFlag{
						Name:  "classpath",
						Usage: "Additional dependency jars for decompilation; guaranteed to be used by CFR fallback retries (repeatable)",
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
					projectConfig, err := resolveProjectConfig(ctx, loadProjectConfig)
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
			},
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
			{
				Name:  "patch-sources",
				Usage: "Patch a JAR by compiling edited Java sources first",
				Flags: []urfavecli.Flag{
					&urfavecli.StringFlag{
						Name:     "input-jar",
						Usage:    "Path to the original input JAR file",
						Required: true,
					},
					&urfavecli.StringFlag{
						Name:     "sources-dir",
						Usage:    "Directory containing Java source files rooted by package path",
						Required: true,
					},
					&urfavecli.StringFlag{
						Name:     "output-jar",
						Usage:    "Path to the patched output JAR file",
						Required: true,
					},
					&urfavecli.StringSliceFlag{
						Name:  "class",
						Usage: "Top-level binary class names to compile and patch (repeatable)",
					},
					&urfavecli.StringFlag{
						Name:  "javac-path",
						Usage: "Path to the javac executable",
					},
					&urfavecli.StringSliceFlag{
						Name:  "classpath",
						Usage: "Additional classpath entries to append after the input JAR (repeatable)",
					},
				},
				Action: func(ctx *urfavecli.Context) error {
					cfg, err := SourcePatchConfigFromContext(ctx)
					if err != nil {
						return err
					}
					projectConfig, err := resolveProjectConfig(ctx, loadProjectConfig)
					if err != nil {
						return err
					}
					cfg = ApplySourcePatchProjectConfig(cfg, projectConfig)
					cfg, err = ValidateSourcePatchConfig(cfg, lookup)
					if err != nil {
						return err
					}
					return sourcePatchRun(ctx.Context, cfg)
				},
			},
		},
	}
}
