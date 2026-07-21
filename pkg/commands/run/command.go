package run

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/sirupsen/logrus"
	"github.com/urfave/cli/v3"

	libconfig "github.com/ekristen/libnuke/pkg/config"
	libnuke "github.com/ekristen/libnuke/pkg/nuke"
	"github.com/ekristen/libnuke/pkg/registry"
	"github.com/ekristen/libnuke/pkg/scanner"
	"github.com/ekristen/libnuke/pkg/types"

	"github.com/entigolabs/oci-nuke/pkg/commands/global"
	"github.com/entigolabs/oci-nuke/pkg/common"
	"github.com/entigolabs/oci-nuke/pkg/nuke"
	"github.com/entigolabs/oci-nuke/pkg/ociutil"

	_ "github.com/entigolabs/oci-nuke/resources"
)

func execute(ctx context.Context, cmd *cli.Command) error {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	region := cmd.String("region")
	compartmentID := cmd.String("compartment-id")
	prefix := cmd.String("prefix")

	oci, err := ociutil.New(ctx, region, compartmentID)
	if err != nil {
		return err
	}

	logger := logrus.StandardLogger()
	logger.SetOutput(os.Stdout)

	params := &libnuke.Parameters{
		Force:              cmd.Bool("no-prompt"),
		ForceSleep:         int(cmd.Int("prompt-delay")),
		Quiet:              cmd.Bool("quiet"),
		NoDryRun:           cmd.Bool("no-dry-run"),
		Includes:           cmd.StringSlice("include"),
		Excludes:           cmd.StringSlice("exclude"),
		WaitOnDependencies: true,
		MaxWaitRetries:     20,
	}

	parsedConfig, err := libconfig.New(libconfig.Options{
		Path:               cmd.String("config"),
		Deprecations:       registry.GetDeprecatedResourceTypeMapping(),
		Log:                logger.WithField("component", "config"),
		NoResolveBlacklist: true,
	})
	if err != nil {
		logger.Errorf("failed to parse config file %s", cmd.String("config"))
		return err
	}

	accountConfig := parsedConfig.Accounts[compartmentID]

	filters, err := parsedConfig.Filters(compartmentID)
	if err != nil {
		return err
	}

	n := libnuke.New(params, filters, parsedConfig.Settings)
	n.SetRunSleep(5 * time.Second)
	n.SetLogger(logger.WithField("component", "libnuke"))
	n.RegisterVersion(fmt.Sprintf("> %s", common.AppVersion))

	p := &nuke.Prompt{Parameters: params, CompartmentID: compartmentID}
	n.RegisterPrompt(p.Prompt)

	opts := &nuke.ListerOpts{
		Provider:      oci.Provider,
		Region:        oci.Region,
		CompartmentID: oci.CompartmentID,
		TenancyID:     oci.TenancyID,
		UserID:        oci.UserID,
		Prefix:        prefix,
	}

	for _, scope := range []registry.Scope{nuke.Compartment, nuke.Tenancy} {
		resourceTypes := types.ResolveResourceTypes(
			registry.GetNamesForScope(scope),
			[]types.Collection{
				n.Parameters.Includes,
				parsedConfig.ResourceTypes.GetIncludes(),
				accountConfig.ResourceTypes.GetIncludes(),
			},
			[]types.Collection{
				n.Parameters.Excludes,
				parsedConfig.ResourceTypes.Excludes,
				accountConfig.ResourceTypes.Excludes,
			},
			nil, nil,
		)

		if len(resourceTypes) == 0 {
			continue
		}

		s, err := scanner.New(&scanner.Config{
			Owner:         region,
			ResourceTypes: resourceTypes,
			Opts:          opts,
			Logger:        logger,
		})
		if err != nil {
			return err
		}

		if err := n.RegisterScanner(scope, s); err != nil {
			return err
		}
	}

	return n.Run(ctx)
}

func init() {
	flags := []cli.Flag{
		&cli.StringFlag{
			Name:  "config",
			Usage: "path to config file",
			Value: "config.yaml",
		},
		&cli.StringSliceFlag{
			Name:  "include",
			Usage: "only include this specific resource",
		},
		&cli.StringSliceFlag{
			Name:  "exclude",
			Usage: "exclude this specific resource (this overrides everything)",
		},
		&cli.BoolFlag{
			Name:    "quiet",
			Aliases: []string{"q"},
			Usage:   "hide filtered messages from display",
		},
		&cli.BoolFlag{
			Name:  "no-dry-run",
			Usage: "actually run the removal of the resources after discovery",
		},
		&cli.BoolFlag{
			Name:  "no-prompt",
			Usage: "disable prompting for verification to run",
		},
		&cli.IntFlag{
			Name:  "prompt-delay",
			Usage: "seconds to delay after prompt before running (minimum: 3 seconds)",
			Value: 10,
		},
		&cli.StringFlag{
			Name:     "compartment-id",
			Usage:    "OCID of the compartment to nuke",
			Sources:  cli.EnvVars("OCI_NUKE_COMPARTMENT_ID"),
			Required: true,
		},
		&cli.StringFlag{
			Name:     "region",
			Usage:    "OCI region the compartment's resources live in",
			Sources:  cli.EnvVars("OCI_REGION"),
			Required: true,
		},
		&cli.StringFlag{
			Name: "prefix",
			Usage: "deployment prefix used to find and filter this deployment's own " +
				"tenancy-wide resources (dynamic groups, policies, secret keys) out of a " +
				"namespace shared with everyone else in the tenancy",
			Sources:  cli.EnvVars("OCI_NUKE_PREFIX"),
			Required: true,
		},
	}

	cmd := &cli.Command{
		Name:    "run",
		Aliases: []string{"nuke"},
		Usage:   "run nuke against an OCI compartment to remove all resources",
		Flags:   append(flags, global.Flags()...),
		Before:  global.Before,
		Action:  execute,
	}

	common.RegisterCommand(cmd)
}
