package common

import (
	"context"

	"github.com/sirupsen/logrus"
	"github.com/urfave/cli/v3"
)

var commands []*cli.Command

// AppVersion is set at build time via -ldflags, defaults to "dev" for local builds.
var AppVersion = "dev"

type Commander interface {
	Execute(ctx context.Context, cmd *cli.Command)
}

func RegisterCommand(command *cli.Command) {
	logrus.Debugln("Registering", command.Name, "command...")
	commands = append(commands, command)
}

func GetCommands() []*cli.Command {
	return commands
}
