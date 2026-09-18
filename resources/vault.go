package resources

import (
	"context"
	"time"

	"github.com/ekristen/libnuke/pkg/registry"
	"github.com/ekristen/libnuke/pkg/resource"
	"github.com/ekristen/libnuke/pkg/types"
	"github.com/oracle/oci-go-sdk/v65/common"
	"github.com/oracle/oci-go-sdk/v65/keymanagement"

	"github.com/entigolabs/oci-nuke/pkg/nuke"
)

const VaultResource = "OCIVault"

// The vault modules/oracle/kms creates to hold the deployment's keys.
//
// Deliberately excluded in config.yaml for the Entigo deployment - see the comment there.
func init() {
	registry.Register(&registry.Registration{
		Name:  VaultResource,
		Scope: nuke.Compartment,
		// Scheduling a vault's deletion takes its keys with it, but doing the keys first
		// keeps the plan readable and means a vault that is excluded while its keys are
		// not still behaves sensibly.
		DependsOn: []string{KeyResource},
		Resource:  &Vault{},
		Lister:    &VaultLister{},
	})
}

// listActiveVaults returns the vaults worth acting on, shared with KeyLister which needs
// each vault's management endpoint before it can see any keys.
func listActiveVaults(ctx context.Context, opts *nuke.ListerOpts) ([]keymanagement.VaultSummary, error) {
	client, err := kmsVaultClient(opts)
	if err != nil {
		return nil, err
	}

	var vaults []keymanagement.VaultSummary
	page := ""
	for {
		resp, err := client.ListVaults(ctx, keymanagement.ListVaultsRequest{
			CompartmentId: &opts.CompartmentID,
			Page:          strPtrOrNil(page),
		})
		if err != nil {
			return nil, err
		}
		for _, v := range resp.Items {
			switch v.LifecycleState {
			case keymanagement.VaultSummaryLifecycleStateDeleted,
				keymanagement.VaultSummaryLifecycleStateDeleting,
				keymanagement.VaultSummaryLifecycleStateSchedulingDeletion,
				keymanagement.VaultSummaryLifecycleStatePendingDeletion:
				continue
			}
			vaults = append(vaults, v)
		}
		if resp.OpcNextPage == nil {
			break
		}
		page = *resp.OpcNextPage
	}
	return vaults, nil
}

type VaultLister struct{}

func (l *VaultLister) List(ctx context.Context, o interface{}) ([]resource.Resource, error) {
	opts := o.(*nuke.ListerOpts)
	vaults, err := listActiveVaults(ctx, opts)
	if err != nil {
		return nil, err
	}
	client, err := kmsVaultClient(opts)
	if err != nil {
		return nil, err
	}

	var resources []resource.Resource
	for _, v := range vaults {
		resources = append(resources, &Vault{
			client:    client,
			ID:        v.Id,
			Name:      v.DisplayName,
			VaultType: string(v.VaultType),
		})
	}
	return resources, nil
}

type Vault struct {
	client    keymanagement.KmsVaultClient
	ID        *string
	Name      *string
	VaultType string
}

// Same 7-to-30-day window as a key, and the same reason for naming the earliest allowed
// time explicitly rather than accepting the 30-day default.
const vaultMinRetention = 7*24*time.Hour + scheduledDeletionSkew

// Same handling as a certificate, for the same reasons - see scheduled_deletion.go.
func (r *Vault) Remove(ctx context.Context) error {
	return deletionSchedule{
		minRetention: vaultMinRetention,
		read: func(ctx context.Context) (string, *common.SDKTime, error) {
			resp, err := r.client.GetVault(ctx, keymanagement.GetVaultRequest{VaultId: r.ID})
			if err != nil {
				return "", nil, err
			}
			return string(resp.LifecycleState), resp.TimeOfDeletion, nil
		},
		schedule: func(ctx context.Context, at time.Time) error {
			_, err := r.client.ScheduleVaultDeletion(ctx, keymanagement.ScheduleVaultDeletionRequest{
				VaultId: r.ID,
				ScheduleVaultDeletionDetails: keymanagement.ScheduleVaultDeletionDetails{
					TimeOfDeletion: &common.SDKTime{Time: at},
				},
			})
			return err
		},
		cancel: func(ctx context.Context) error {
			_, err := r.client.CancelVaultDeletion(ctx, keymanagement.CancelVaultDeletionRequest{VaultId: r.ID})
			return err
		},
	}.ensure(ctx)
}

func (r *Vault) Properties() types.Properties {
	return types.NewPropertiesFromStruct(r)
}

func (r *Vault) String() string {
	if r.Name != nil {
		return *r.Name
	}
	return *r.ID
}
