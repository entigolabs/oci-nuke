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

const DynamicGroupResource = "OCIDynamicGroup"

func init() {
	registry.Register(&registry.Registration{
		Name:     DynamicGroupResource,
		Scope:    nuke.Tenancy,
		Resource: &DynamicGroup{},
		Lister:   &DynamicGroupLister{},
	})
}

type DynamicGroupLister struct{}

// Dynamic groups live in the tenancy's root compartment - a namespace shared with
// everyone else in the org - so only ones matching this deployment's Prefix are
// returned. Unlike Compartment-scoped resources, it would be unsafe to sweep all of
// these unconditionally.
func (l *DynamicGroupLister) List(ctx context.Context, o interface{}) ([]resource.Resource, error) {
	opts := o.(*nuke.ListerOpts)
	if opts.Prefix == "" {
		return nil, nil
	}
	client, err := identityClient(opts)
	if err != nil {
		return nil, err
	}

	resp, err := client.ListDynamicGroups(ctx, identity.ListDynamicGroupsRequest{
		CompartmentId: &opts.TenancyID,
	})
	if err != nil {
		return nil, err
	}

	var resources []resource.Resource
	for _, dg := range resp.Items {
		if dg.LifecycleState == identity.DynamicGroupLifecycleStateDeleted || dg.LifecycleState == identity.DynamicGroupLifecycleStateDeleting {
			continue
		}
		if dg.Name == nil || !strings.HasPrefix(*dg.Name, opts.Prefix+"-") {
			continue
		}
		resources = append(resources, &DynamicGroup{client: client, ID: dg.Id, Name: dg.Name})
	}
	return resources, nil
}

type DynamicGroup struct {
	client identity.IdentityClient
	ID     *string
	Name   *string
}

func (r *DynamicGroup) Remove(ctx context.Context) error {
	_, err := r.client.DeleteDynamicGroup(ctx, identity.DeleteDynamicGroupRequest{DynamicGroupId: r.ID})
	return err
}

func (r *DynamicGroup) Properties() types.Properties {
	return types.NewPropertiesFromStruct(r)
}

func (r *DynamicGroup) String() string {
	if r.Name != nil {
		return *r.Name
	}
	return *r.ID
}
