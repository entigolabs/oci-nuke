package resources

import (
	"context"
	"strings"

	"github.com/ekristen/libnuke/pkg/registry"
	"github.com/ekristen/libnuke/pkg/resource"
	"github.com/ekristen/libnuke/pkg/types"
	"github.com/oracle/oci-go-sdk/v65/identity"

	"github.com/entigolabs/oci-nuke/pkg/nuke"
)

const CustomerSecretKeyResource = "OCICustomerSecretKey"

func init() {
	registry.Register(&registry.Registration{
		Name:     CustomerSecretKeyResource,
		Scope:    nuke.Tenancy,
		Resource: &CustomerSecretKey{},
		Lister:   &CustomerSecretKeyLister{},
	})
}

type CustomerSecretKeyLister struct{}

// Customer secret keys are per-user, not per-tenancy, so this only ever touches your
// own keys (never lists other users). Skips entirely under resource-principal auth,
// where there's no OCI user. Filtered by "entigo-infralib-<prefix>" display name,
// matching oracle/oracle.go's CreateServiceAccount / getBucketResources naming.
func (l *CustomerSecretKeyLister) List(ctx context.Context, o interface{}) ([]resource.Resource, error) {
	opts := o.(*nuke.ListerOpts)
	if opts.Prefix == "" || opts.UserID == "" {
		return nil, nil
	}
	client, err := identityClient(opts)
	if err != nil {
		return nil, err
	}

	resp, err := client.ListCustomerSecretKeys(ctx, identity.ListCustomerSecretKeysRequest{
		UserId: &opts.UserID,
	})
	if err != nil {
		return nil, err
	}

	wantPrefix := "entigo-infralib-" + opts.Prefix
	var resources []resource.Resource
	for _, key := range resp.Items {
		if key.LifecycleState == identity.CustomerSecretKeySummaryLifecycleStateDeleting {
			continue
		}
		if key.DisplayName == nil || !strings.HasPrefix(*key.DisplayName, wantPrefix) {
			continue
		}
		resources = append(resources, &CustomerSecretKey{client: client, UserID: opts.UserID, ID: key.Id, Name: key.DisplayName})
	}
	return resources, nil
}

type CustomerSecretKey struct {
	client identity.IdentityClient
	UserID string
	ID     *string
	Name   *string
}

func (r *CustomerSecretKey) Remove(ctx context.Context) error {
	_, err := r.client.DeleteCustomerSecretKey(ctx, identity.DeleteCustomerSecretKeyRequest{
		UserId:              &r.UserID,
		CustomerSecretKeyId: r.ID,
	})
	return err
}

func (r *CustomerSecretKey) Properties() types.Properties {
	return types.NewPropertiesFromStruct(r)
}

func (r *CustomerSecretKey) String() string {
	if r.Name != nil {
		return *r.Name
	}
	return *r.ID
}
