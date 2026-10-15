package nuke

import (
	"fmt"
	"strings"

	"github.com/ekristen/libnuke/pkg/registry"
	ocicommon "github.com/oracle/oci-go-sdk/v65/common"
)

// Compartment-scoped resources are swept unconditionally within the target compartment.
// Tenancy-scoped resources (IAM: dynamic groups, policies, users' secret keys) live in a
// namespace shared across the whole tenancy and must be filtered by Prefixes instead.
const (
	Compartment registry.Scope = "compartment"
	Tenancy     registry.Scope = "tenancy"
)

// ListerOpts is passed to every resource Lister. CompartmentID is the sweep target;
// TenancyID/UserID/Prefixes are only used by Tenancy-scoped resources to find and filter
// this deployment's own resources out of a namespace shared with everyone else.
type ListerOpts struct {
	Provider      ocicommon.ConfigurationProvider
	Region        string
	CompartmentID string
	TenancyID     string
	UserID        string
	Prefixes      []string
}

// MatchesPrefix reports whether name starts with format filled in with any of Prefixes.
func (o *ListerOpts) MatchesPrefix(name *string, format string) bool {
	if name == nil {
		return false
	}
	for _, p := range o.Prefixes {
		if strings.HasPrefix(*name, fmt.Sprintf(format, p)) {
			return true
		}
	}
	return false
}
