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

// Same stage-first requirement and leaves-first ordering as BuildPipeline.Remove - see
// the comment there.
func (r *DeployPipeline) Remove(ctx context.Context) error {
	for {
		var stageIDs []*string
		page := ""
		for {
			resp, err := r.client.ListDeployStages(ctx, devops.ListDeployStagesRequest{
				DeployPipelineId: r.ID,
				Page:             strPtrOrNil(page),
			})
			if err != nil {
				return err
			}
			for _, s := range resp.Items {
				switch s.GetLifecycleState() {
				case devops.DeployStageLifecycleStateDeleted, devops.DeployStageLifecycleStateDeleting:
					continue
				}
				stageIDs = append(stageIDs, s.GetId())
			}
			if resp.OpcNextPage == nil {
				break
			}
			page = *resp.OpcNextPage
		}

		if len(stageIDs) == 0 {
			break
		}

		progress := false
		var lastErr error
		for _, id := range stageIDs {
			if _, err := r.client.DeleteDeployStage(ctx, devops.DeleteDeployStageRequest{DeployStageId: id}); err != nil {
				lastErr = err
				continue
			}
			progress = true
		}
		if !progress {
			return lastErr
		}
	}

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
