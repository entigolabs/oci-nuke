package resources

import (
	"context"

	"github.com/ekristen/libnuke/pkg/registry"
	"github.com/ekristen/libnuke/pkg/resource"
	"github.com/ekristen/libnuke/pkg/types"
	"github.com/oracle/oci-go-sdk/v65/devops"

	"github.com/entigolabs/oci-nuke/pkg/nuke"
)

const DevopsProjectResource = "OCIDevopsProject"

func init() {
	registry.Register(&registry.Registration{
		Name:      DevopsProjectResource,
		Scope:     nuke.Compartment,
		Resource:  &DevopsProject{},
		Lister:    &DevopsProjectLister{},
		DependsOn: []string{DeployPipelineResource, BuildPipelineResource},
	})
}

type DevopsProjectLister struct{}

func (l *DevopsProjectLister) List(ctx context.Context, o interface{}) ([]resource.Resource, error) {
	opts := o.(*nuke.ListerOpts)
	client, err := devopsClient(opts)
	if err != nil {
		return nil, err
	}

	var resources []resource.Resource
	page := ""
	for {
		resp, err := client.ListProjects(ctx, devops.ListProjectsRequest{
			CompartmentId: &opts.CompartmentID,
			Page:          strPtrOrNil(page),
		})
		if err != nil {
			return nil, err
		}
		for _, p := range resp.Items {
			switch p.LifecycleState {
			case devops.ProjectLifecycleStateDeleted, devops.ProjectLifecycleStateDeleting:
				continue
			}
			resources = append(resources, &DevopsProject{client: client, ID: p.Id, Name: p.Name})
		}
		if resp.OpcNextPage == nil {
			break
		}
		page = *resp.OpcNextPage
	}
	return resources, nil
}

type DevopsProject struct {
	client devops.DevopsClient
	ID     *string
	Name   *string
}

func (r *DevopsProject) Remove(ctx context.Context) error {
	_, err := r.client.DeleteProject(ctx, devops.DeleteProjectRequest{ProjectId: r.ID})
	return err
}

func (r *DevopsProject) Properties() types.Properties {
	return types.NewPropertiesFromStruct(r)
}

func (r *DevopsProject) String() string {
	if r.Name != nil {
		return *r.Name
	}
	return *r.ID
}
