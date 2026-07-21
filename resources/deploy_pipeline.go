package resources

import (
	"context"

	"github.com/ekristen/libnuke/pkg/registry"
	"github.com/ekristen/libnuke/pkg/resource"
	"github.com/ekristen/libnuke/pkg/types"
	"github.com/oracle/oci-go-sdk/v65/devops"

	"github.com/entigolabs/oci-nuke/pkg/nuke"
)

const DeployPipelineResource = "OCIDeployPipeline"

func init() {
	registry.Register(&registry.Registration{
		Name:     DeployPipelineResource,
		Scope:    nuke.Compartment,
		Resource: &DeployPipeline{},
		Lister:   &DeployPipelineLister{},
	})
}

type DeployPipelineLister struct{}

func (l *DeployPipelineLister) List(ctx context.Context, o interface{}) ([]resource.Resource, error) {
	opts := o.(*nuke.ListerOpts)
	client, err := devopsClient(opts)
	if err != nil {
		return nil, err
	}

	var resources []resource.Resource
	page := ""
	for {
		resp, err := client.ListDeployPipelines(ctx, devops.ListDeployPipelinesRequest{
			CompartmentId: &opts.CompartmentID,
			Page:          strPtrOrNil(page),
		})
		if err != nil {
			return nil, err
		}
		for _, p := range resp.Items {
			switch p.LifecycleState {
			case devops.DeployPipelineLifecycleStateDeleted, devops.DeployPipelineLifecycleStateDeleting:
				continue
			}
			resources = append(resources, &DeployPipeline{client: client, ID: p.Id, Name: p.DisplayName})
		}
		if resp.OpcNextPage == nil {
			break
		}
		page = *resp.OpcNextPage
	}
	return resources, nil
}

type DeployPipeline struct {
	client devops.DevopsClient
	ID     *string
	Name   *string
}

func (r *DeployPipeline) Remove(ctx context.Context) error {
	_, err := r.client.DeleteDeployPipeline(ctx, devops.DeleteDeployPipelineRequest{DeployPipelineId: r.ID})
	return err
}

func (r *DeployPipeline) Properties() types.Properties {
	return types.NewPropertiesFromStruct(r)
}

func (r *DeployPipeline) String() string {
	if r.Name != nil {
		return *r.Name
	}
	return *r.ID
}
