package main

import (
	"context"
	"os"
	"path"

	"github.com/sirupsen/logrus"
	"github.com/urfave/cli/v3"

	"github.com/entigolabs/oci-nuke/pkg/common"

	_ "github.com/entigolabs/oci-nuke/pkg/commands/run"
)

func main() {
	defer func() {
		if r := recover(); r != nil {
			if _, ok := r.(*logrus.Entry); ok {
				os.Exit(1)
			}
			panic(r)
		}
	}()

	cmd := &cli.Command{
		Name:    path.Base(os.Args[0]),
		Usage:   "remove everything from an OCI compartment",
		Version: common.AppVersion,
		Authors: []any{
			"Entigo",
		},
		Commands: common.GetCommands(),
		CommandNotFound: func(_ context.Context, _ *cli.Command, command string) {
			logrus.Fatalf("command %s not found.", command)
		},
	}

	if err := cmd.Run(context.Background(), os.Args); err != nil {
		logrus.Fatal(err)
	}
}
