package nuke

import (
	"fmt"
	"time"

	libnuke "github.com/ekristen/libnuke/pkg/nuke"
	"github.com/ekristen/libnuke/pkg/utils"
)

type Prompt struct {
	Parameters    *libnuke.Parameters
	CompartmentID string
}

// Prompt is the function libnuke calls before running a non-dry-run nuke.
func (p *Prompt) Prompt() error {
	promptDelay := time.Duration(p.Parameters.ForceSleep) * time.Second

	fmt.Printf("Do you really want to nuke the compartment with the ID '%s'?\n", p.CompartmentID)
	if p.Parameters.Force {
		fmt.Printf("Waiting %v before continuing.\n", promptDelay)
		time.Sleep(promptDelay)
	} else {
		fmt.Printf("Do you want to continue? Enter the compartment ID to continue.\n")
		if err := utils.Prompt(p.CompartmentID); err != nil {
			return err
		}
	}

	return nil
}
