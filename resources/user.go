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

const UserResource = "OCIUser"

func init() {
	registry.Register(&registry.Registration{
		Name:     UserResource,
		Scope:    nuke.Tenancy,
		Resource: &User{},
		Lister:   &UserLister{},
	})
}

type UserLister struct{}

// Users live in the tenancy's root compartment - a namespace shared with everyone else in the
// organisation - so only ones matching this deployment's Prefix are returned, the same caveat
// as dynamic_group.go.
//
// entigo-infralib creates one of these because loki cannot use instance principal: it reaches
// Object Storage through the S3-compatibility endpoint, which authenticates only with a
// Customer Secret Key, and that is a credential of a user. Deleting the user takes its secret
// keys with it, which is just as well - customer_secret_key.go can only see keys belonging to
// the caller's own user, never this one's.
func (l *UserLister) List(ctx context.Context, o interface{}) ([]resource.Resource, error) {
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
		resp, err := client.ListUsers(ctx, identity.ListUsersRequest{
			CompartmentId: &opts.TenancyID,
			Page:          strPtrOrNil(page),
		})
		if err != nil {
			return nil, err
		}
		for _, u := range resp.Items {
			if u.LifecycleState == identity.UserLifecycleStateDeleted || u.LifecycleState == identity.UserLifecycleStateDeleting {
				continue
			}
			if u.Name == nil || !strings.HasPrefix(*u.Name, opts.Prefix+"-") {
				continue
			}
			resources = append(resources, &User{client: client, ID: u.Id, Name: u.Name})
		}
		if resp.OpcNextPage == nil {
			break
		}
		page = *resp.OpcNextPage
	}
	return resources, nil
}

type User struct {
	client identity.IdentityClient
	ID     *string
	Name   *string
}

func (r *User) Remove(ctx context.Context) error {
	_, err := r.client.DeleteUser(ctx, identity.DeleteUserRequest{UserId: r.ID})
	return err
}

func (r *User) Properties() types.Properties {
	return types.NewPropertiesFromStruct(r)
}

func (r *User) String() string {
	if r.Name != nil {
		return *r.Name
	}
	return *r.ID
}
