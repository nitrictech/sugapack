package main

import (
	"context"
	"fmt"
	"os"

	"github.com/moby/buildkit/frontend/gateway/grpcclient"
	"github.com/sugapack/sugapack/buildkit"
	"github.com/sugapack/sugapack/planner"
	cli "github.com/urfave/cli/v3"
)

func main() {
	app := &cli.Command{
		Name:  "sugapack",
		Usage: "BuildKit frontend wrapping railpack with remote git source support",
		Commands: []*cli.Command{
			{
				Name:  "frontend",
				Usage: "Start the BuildKit gRPC frontend server",
				Action: func(ctx context.Context, cmd *cli.Command) error {
					return grpcclient.RunFromEnvironment(ctx, buildkit.Build)
				},
			},
			{
				Name:      "plan",
				Usage:     "Generate a railpack build plan for a directory",
				ArgsUsage: "DIRECTORY",
				Flags: []cli.Flag{
					&cli.StringFlag{
						Name:    "out",
						Aliases: []string{"o"},
						Value:   "/out/plan.json",
						Usage:   "output file for the plan JSON",
					},
					&cli.StringFlag{
						Name:  "build-cmd",
						Usage: "build command override",
					},
					&cli.StringFlag{
						Name:  "start-cmd",
						Usage: "start command override",
					},
					&cli.StringSliceFlag{
						Name:    "env",
						Aliases: []string{"e"},
						Usage:   "legacy environment variables (KEY=VAL); names stay plan secrets",
					},
					&cli.StringSliceFlag{
						Name:  "build-variable",
						Usage: "environment variables (KEY=VAL) for the build steps only",
					},
					&cli.StringSliceFlag{
						Name:  "variable",
						Usage: "environment variables (KEY=VAL) baked into the final image",
					},
					&cli.StringSliceFlag{
						Name:  "secret",
						Usage: "name of a build-time secret; the value is supplied by BuildKit",
					},
				},
				Action: func(ctx context.Context, cmd *cli.Command) error {
					sourceDir := cmd.Args().First()
					if sourceDir == "" {
						return fmt.Errorf("source directory is required")
					}
					return planner.Run(planner.Options{
						SourceDir:      sourceDir,
						OutputFile:     cmd.String("out"),
						BuildCmd:       cmd.String("build-cmd"),
						StartCmd:       cmd.String("start-cmd"),
						Envs:           cmd.StringSlice("env"),
						BuildVariables: cmd.StringSlice("build-variable"),
						Variables:      cmd.StringSlice("variable"),
						Secrets:        cmd.StringSlice("secret"),
					})
				},
			},
		},
		// Default action (no subcommand): run as frontend.
		// This handles the case where BuildKit invokes the binary directly.
		Action: func(ctx context.Context, cmd *cli.Command) error {
			return grpcclient.RunFromEnvironment(ctx, buildkit.Build)
		},
	}

	if err := app.Run(context.Background(), os.Args); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
}
