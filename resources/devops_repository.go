package resources

import (
	"context"

	"github.com/ekristen/libnuke/pkg/registry"
	"github.com/ekristen/libnuke/pkg/resource"
	"github.com/ekristen/libnuke/pkg/types"
	"github.com/oracle/oci-go-sdk/v65/devops"

	"github.com/entigolabs/oci-nuke/pkg/nuke"
)

const DevopsRepositoryResource = "OCIDevopsRepository"

// The agent creates a DevOps code repository per project (`<prefix>-infralib-src`, holding
// the generated build specs) and OCI refuses to delete a project that still contains one,
// so without this type the whole DevOps chain survives a nuke.
func init() {
	registry.Register(&registry.Registration{
		Name:     DevopsRepositoryResource,
		Scope:    nuke.Compartment,
		Resource: &DevopsRepository{},
		Lister:   &DevopsRepositoryLister{},
	})
}

type DevopsRepositoryLister struct{}

func (l *DevopsRepositoryLister) List(ctx context.Context, o interface{}) ([]resource.Resource, error) {
	opts := o.(*nuke.ListerOpts)
	client, err := devopsClient(opts)
	if err != nil {
		return nil, err
	}

	var resources []resource.Resource
	page := ""
	for {
		resp, err := client.ListRepositories(ctx, devops.ListRepositoriesRequest{
			CompartmentId: &opts.CompartmentID,
			Page:          strPtrOrNil(page),
		})
		if err != nil {
			return nil, err
		}
		for _, r := range resp.Items {
			switch r.LifecycleState {
			case devops.RepositoryLifecycleStateDeleted, devops.RepositoryLifecycleStateDeleting:
				continue
			}
			resources = append(resources, &DevopsRepository{client: client, ID: r.Id, Name: r.Name})
		}
		if resp.OpcNextPage == nil {
			break
		}
		page = *resp.OpcNextPage
	}
	return resources, nil
}

type DevopsRepository struct {
	client devops.DevopsClient
	ID     *string
	Name   *string
}

func (r *DevopsRepository) Remove(ctx context.Context) error {
	_, err := r.client.DeleteRepository(ctx, devops.DeleteRepositoryRequest{RepositoryId: r.ID})
	return err
}

func (r *DevopsRepository) Properties() types.Properties {
	return types.NewPropertiesFromStruct(r)
}

func (r *DevopsRepository) String() string {
	if r.Name != nil {
		return *r.Name
	}
	return *r.ID
}
