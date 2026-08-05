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

const GroupResource = "OCIGroup"

func init() {
	registry.Register(&registry.Registration{
		Name:     GroupResource,
		Scope:    nuke.Tenancy,
		Resource: &Group{},
		// Users are removed first: OCI refuses to delete a group that still has members, and
		// deleting the user takes its memberships with it.
		DependsOn: []string{UserResource},
		Lister:    &GroupLister{},
	})
}

type GroupLister struct{}

// Same shared-namespace caveat as dynamic_group.go: tenancy root, so filtered by Prefix.
// entigo-infralib creates one only because an OCI policy cannot name an individual user - it
// grants to groups - so loki's user needs a group to sit in.
func (l *GroupLister) List(ctx context.Context, o interface{}) ([]resource.Resource, error) {
	opts := o.(*nuke.ListerOpts)
	if opts.Prefix == "" {
		return nil, nil
	}
	client, err := identityClient(opts)
	if err != nil {
		return nil, err
	}

	var resources []resource.Resource
	page := ""
	for {
		resp, err := client.ListGroups(ctx, identity.ListGroupsRequest{
			CompartmentId: &opts.TenancyID,
			Page:          strPtrOrNil(page),
		})
		if err != nil {
			return nil, err
		}
		for _, g := range resp.Items {
			if g.LifecycleState == identity.GroupLifecycleStateDeleted || g.LifecycleState == identity.GroupLifecycleStateDeleting {
				continue
			}
			if g.Name == nil || !strings.HasPrefix(*g.Name, opts.Prefix+"-") {
				continue
			}
			resources = append(resources, &Group{client: client, ID: g.Id, Name: g.Name})
		}
		if resp.OpcNextPage == nil {
			break
		}
		page = *resp.OpcNextPage
	}
	return resources, nil
}

type Group struct {
	client identity.IdentityClient
	ID     *string
	Name   *string
}

func (r *Group) Remove(ctx context.Context) error {
	_, err := r.client.DeleteGroup(ctx, identity.DeleteGroupRequest{GroupId: r.ID})
	return err
}

func (r *Group) Properties() types.Properties {
	return types.NewPropertiesFromStruct(r)
}

func (r *Group) String() string {
	if r.Name != nil {
		return *r.Name
	}
	return *r.ID
}
