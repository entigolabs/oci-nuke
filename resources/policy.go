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

const PolicyResource = "OCIPolicy"

func init() {
	registry.Register(&registry.Registration{
		Name:     PolicyResource,
		Scope:    nuke.Tenancy,
		Resource: &Policy{},
		Lister:   &PolicyLister{},
	})
}

type PolicyLister struct{}

// Same shared-namespace caveat as dynamic_group.go: filtered by Prefix, not swept
// unconditionally.
func (l *PolicyLister) List(ctx context.Context, o interface{}) ([]resource.Resource, error) {
	opts := o.(*nuke.ListerOpts)
	if opts.Prefix == "" {
		return nil, nil
	}
	client, err := identityClient(opts)
	if err != nil {
		return nil, err
	}

	resp, err := client.ListPolicies(ctx, identity.ListPoliciesRequest{
		CompartmentId: &opts.TenancyID,
	})
	if err != nil {
		return nil, err
	}

	var resources []resource.Resource
	for _, p := range resp.Items {
		if p.LifecycleState == identity.PolicyLifecycleStateDeleted || p.LifecycleState == identity.PolicyLifecycleStateDeleting {
			continue
		}
		if p.Name == nil || !strings.HasPrefix(*p.Name, opts.Prefix+"-") {
			continue
		}
		resources = append(resources, &Policy{client: client, ID: p.Id, Name: p.Name})
	}
	return resources, nil
}

type Policy struct {
	client identity.IdentityClient
	ID     *string
	Name   *string
}

func (r *Policy) Remove(ctx context.Context) error {
	_, err := r.client.DeletePolicy(ctx, identity.DeletePolicyRequest{PolicyId: r.ID})
	return err
}

func (r *Policy) Properties() types.Properties {
	return types.NewPropertiesFromStruct(r)
}

func (r *Policy) String() string {
	if r.Name != nil {
		return *r.Name
	}
	return *r.ID
}
