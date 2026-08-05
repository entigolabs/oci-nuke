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

// Policies are the one IAM type that shows up in both scopes, so this lister sweeps both.
//
//   - In the tenancy's root compartment they share a namespace with the whole org, so the
//     same caveat as dynamic_group.go applies: only ones matching Prefix are returned.
//   - In the target compartment they are this deployment's own, so they are swept
//     unconditionally like every other compartment-scoped resource. Prefix filtering would
//     be actively wrong there: the per-app policies Crossplane creates are named after
//     their Helm release ("external-dns", "cluster-autoscaler"), carrying no prefix at all,
//     and leaving them behind makes the next provision fail - Crossplane cannot recreate a
//     policy whose name is already taken in that compartment.
func (l *PolicyLister) List(ctx context.Context, o interface{}) ([]resource.Resource, error) {
	opts := o.(*nuke.ListerOpts)
	client, err := identityClient(opts)
	if err != nil {
		return nil, err
	}

	var resources []resource.Resource

	if opts.Prefix != "" {
		tenancyPolicies, err := listPolicies(ctx, client, opts.TenancyID)
		if err != nil {
			return nil, err
		}
		for _, p := range tenancyPolicies {
			if p.Name == nil || !strings.HasPrefix(*p.Name, opts.Prefix+"-") {
				continue
			}
			resources = append(resources, &Policy{client: client, ID: p.Id, Name: p.Name})
		}
	}

	// Skip when the sweep target *is* the tenancy root, or every policy would be listed
	// twice - and the second time without the prefix filter protecting the rest of the org.
	if opts.CompartmentID != opts.TenancyID {
		compartmentPolicies, err := listPolicies(ctx, client, opts.CompartmentID)
		if err != nil {
			return nil, err
		}
		for _, p := range compartmentPolicies {
			resources = append(resources, &Policy{client: client, ID: p.Id, Name: p.Name})
		}
	}

	return resources, nil
}

func listPolicies(ctx context.Context, client identity.IdentityClient, compartmentID string) ([]identity.Policy, error) {
	var policies []identity.Policy
	page := ""
	for {
		resp, err := client.ListPolicies(ctx, identity.ListPoliciesRequest{
			CompartmentId: &compartmentID,
			Page:          strPtrOrNil(page),
		})
		if err != nil {
			return nil, err
		}
		for _, p := range resp.Items {
			if p.LifecycleState == identity.PolicyLifecycleStateDeleted || p.LifecycleState == identity.PolicyLifecycleStateDeleting {
				continue
			}
			policies = append(policies, p)
		}
		if resp.OpcNextPage == nil {
			break
		}
		page = *resp.OpcNextPage
	}
	return policies, nil
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
