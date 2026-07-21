// Package ociutil resolves OCI credentials and account context, mirroring how the
// entigo-infralib-agent and the oci CLI do it: resource principal when running inside
// an OCI Container Instance, otherwise the SDK default chain (~/.oci/config, honoring
// OCI_CONFIG_FILE, or config env vars).
package ociutil

import (
	"context"
	"fmt"
	"os"

	ocicommon "github.com/oracle/oci-go-sdk/v65/common"
	"github.com/oracle/oci-go-sdk/v65/common/auth"
	"github.com/oracle/oci-go-sdk/v65/identity"
)

// OCI holds resolved account context shared by every resource type.
type OCI struct {
	Provider      ocicommon.ConfigurationProvider
	Region        string
	CompartmentID string
	TenancyID     string
	// UserID is empty when authenticating via resource principal - there's no OCI
	// user in that case, so user-scoped resources (customer secret keys) are skipped.
	UserID string
}

func newConfigProvider() (ocicommon.ConfigurationProvider, error) {
	if os.Getenv(auth.ResourcePrincipalVersionEnvVar) != "" {
		return auth.ResourcePrincipalConfigurationProvider()
	}
	return ocicommon.DefaultConfigProvider(), nil
}

// New resolves credentials and validates the compartment exists and is reachable.
func New(ctx context.Context, region, compartmentID string) (*OCI, error) {
	if compartmentID == "" {
		return nil, fmt.Errorf("compartment id must be set")
	}

	provider, err := newConfigProvider()
	if err != nil {
		return nil, fmt.Errorf("failed to build OCI configuration provider: %w", err)
	}

	tenancyID, err := provider.TenancyOCID()
	if err != nil {
		return nil, fmt.Errorf("failed to resolve tenancy OCID: %w", err)
	}

	// UserOCID is absent under resource principal auth - not an error, just means
	// user-scoped resource types will skip themselves.
	userID, _ := provider.UserOCID()

	o := &OCI{
		Provider:      provider,
		Region:        region,
		CompartmentID: compartmentID,
		TenancyID:     tenancyID,
		UserID:        userID,
	}

	client, err := identity.NewIdentityClientWithConfigurationProvider(provider)
	if err != nil {
		return nil, fmt.Errorf("failed to build identity client: %w", err)
	}
	if region != "" {
		client.SetRegion(region)
	}
	if _, err := client.GetCompartment(ctx, identity.GetCompartmentRequest{CompartmentId: &compartmentID}); err != nil {
		return nil, fmt.Errorf("failed to look up compartment %s: %w", compartmentID, err)
	}

	return o, nil
}
