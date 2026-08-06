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

const KeyResource = "OCIKey"

// Master encryption keys from modules/oracle/kms - data, config, telemetry, and the CA
// signing key.
//
// Deliberately excluded in config.yaml for the Entigo deployment - see the comment there.
// Keys are the reason the exclusion exists at all: an HSM-protected key version cannot be
// deleted, only scheduled 7 days out, and it is billed for the whole of that wait.
func init() {
	registry.Register(&registry.Registration{
		Name:  KeyResource,
		Scope: nuke.Compartment,
		// The CA holds a reference to its signing key, so it goes first.
		DependsOn: []string{CertificateAuthorityResource},
		Resource:  &Key{},
		Lister:    &KeyLister{},
	})
}

type KeyLister struct{}

// Keys are not listed against the regional endpoint: every vault has its own management
// endpoint and only that endpoint knows the vault's keys. So this enumerates vaults first
// and then asks each one, which is also why OCIVault and OCIKey cannot share a client.
func (l *KeyLister) List(ctx context.Context, o interface{}) ([]resource.Resource, error) {
	opts := o.(*nuke.ListerOpts)
	vaults, err := listActiveVaults(ctx, opts)
	if err != nil {
		return nil, err
	}

	var resources []resource.Resource
	for _, v := range vaults {
		if v.ManagementEndpoint == nil {
			continue
		}
		client, err := kmsManagementClient(opts, *v.ManagementEndpoint)
		if err != nil {
			return nil, err
		}

		page := ""
		for {
			resp, err := client.ListKeys(ctx, keymanagement.ListKeysRequest{
				CompartmentId: &opts.CompartmentID,
				Page:          strPtrOrNil(page),
			})
			if err != nil {
				return nil, err
			}
			for _, k := range resp.Items {
				switch k.LifecycleState {
				case keymanagement.KeySummaryLifecycleStateDeleted,
					keymanagement.KeySummaryLifecycleStateDeleting,
					keymanagement.KeySummaryLifecycleStateSchedulingDeletion,
					keymanagement.KeySummaryLifecycleStatePendingDeletion:
					continue
				}
				resources = append(resources, &Key{
					client:    client,
					ID:        k.Id,
					Name:      k.DisplayName,
					VaultName: v.DisplayName,
				})
			}
			if resp.OpcNextPage == nil {
				break
			}
			page = *resp.OpcNextPage
		}
	}
	return resources, nil
}

type Key struct {
	client    keymanagement.KmsManagementClient
	ID        *string
	Name      *string
	VaultName *string
}

// keyMinRetention is the floor OCI puts on a scheduled key deletion: "the specified time
// must be between 7 and 30 days from when the request is received". An hour of margin
// keeps a slow request or a skewed clock off the boundary. Without an explicit time the
// default is the 30-day maximum, which is the worst outcome for a key that is billed
// while it waits.
const keyMinRetention = 7*24*time.Hour + time.Hour

func (r *Key) Remove(ctx context.Context) error {
	deleteAt := time.Now().Add(keyMinRetention)
	_, err := r.client.ScheduleKeyDeletion(ctx, keymanagement.ScheduleKeyDeletionRequest{
		KeyId: r.ID,
		ScheduleKeyDeletionDetails: keymanagement.ScheduleKeyDeletionDetails{
			TimeOfDeletion: &common.SDKTime{Time: deleteAt},
		},
	})
	return err
}

func (r *Key) Properties() types.Properties {
	return types.NewPropertiesFromStruct(r)
}

func (r *Key) String() string {
	if r.Name != nil {
		return *r.Name
	}
	return *r.ID
}
