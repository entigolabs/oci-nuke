package nuke

import (
	"github.com/ekristen/libnuke/pkg/registry"
	ocicommon "github.com/oracle/oci-go-sdk/v65/common"
)

// Compartment-scoped resources are swept unconditionally within the target compartment.
// Tenancy-scoped resources (IAM: dynamic groups, policies, users' secret keys) live in a
// namespace shared across the whole tenancy and must be filtered by Prefix instead.
const (
	Compartment registry.Scope = "compartment"
	Tenancy     registry.Scope = "tenancy"
)

// ListerOpts is passed to every resource Lister. CompartmentID is the sweep target;
// TenancyID/UserID/Prefix are only used by Tenancy-scoped resources to find and filter
// this deployment's own resources out of a namespace shared with everyone else.
type ListerOpts struct {
	Provider      ocicommon.ConfigurationProvider
	Region        string
	CompartmentID string
	TenancyID     string
	UserID        string
	Prefix        string
}
