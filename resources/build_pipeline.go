package resources

import (
	"context"

	"github.com/ekristen/libnuke/pkg/registry"
	"github.com/ekristen/libnuke/pkg/resource"
	"github.com/ekristen/libnuke/pkg/types"
	"github.com/oracle/oci-go-sdk/v65/devops"

	"github.com/entigolabs/oci-nuke/pkg/nuke"
)

const BuildPipelineResource = "OCIBuildPipeline"

func init() {
	registry.Register(&registry.Registration{
		Name:     BuildPipelineResource,
		Scope:    nuke.Compartment,
		Resource: &BuildPipeline{},
		Lister:   &BuildPipelineLister{},
	})
}

type BuildPipelineLister struct{}

func (l *BuildPipelineLister) List(ctx context.Context, o interface{}) ([]resource.Resource, error) {
	opts := o.(*nuke.ListerOpts)
	client, err := devopsClient(opts)
	if err != nil {
		return nil, err
	}

	var resources []resource.Resource
	page := ""
	for {
		resp, err := client.ListBuildPipelines(ctx, devops.ListBuildPipelinesRequest{
			CompartmentId: &opts.CompartmentID,
			Page:          strPtrOrNil(page),
		})
		if err != nil {
			return nil, err
		}
		for _, p := range resp.Items {
			switch p.LifecycleState {
			case devops.BuildPipelineLifecycleStateDeleted, devops.BuildPipelineLifecycleStateDeleting:
				continue
			}
			resources = append(resources, &BuildPipeline{client: client, ID: p.Id, Name: p.DisplayName})
		}
		if resp.OpcNextPage == nil {
			break
		}
		page = *resp.OpcNextPage
	}
	return resources, nil
}

type BuildPipeline struct {
	client devops.DevopsClient
	ID     *string
	Name   *string
}

// OCI refuses to delete a build pipeline that still has stages, and stages themselves
// can only be deleted leaves-first (a stage with successors rejects deletion) - the same
// order the manual pipeline-recreate procedure needs. Multiple passes handle the
// ordering without reconstructing the stage graph: each pass deletes whatever currently
// has no dependents, and a pass with zero progress means something is genuinely stuck.
func (r *BuildPipeline) Remove(ctx context.Context) error {
	for {
		var stageIDs []*string
		page := ""
		for {
			resp, err := r.client.ListBuildPipelineStages(ctx, devops.ListBuildPipelineStagesRequest{
				BuildPipelineId: r.ID,
				Page:            strPtrOrNil(page),
			})
			if err != nil {
				return err
			}
			for _, s := range resp.Items {
				switch s.GetLifecycleState() {
				case devops.BuildPipelineStageLifecycleStateDeleted, devops.BuildPipelineStageLifecycleStateDeleting:
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
			if _, err := r.client.DeleteBuildPipelineStage(ctx, devops.DeleteBuildPipelineStageRequest{BuildPipelineStageId: id}); err != nil {
				lastErr = err
				continue
			}
			progress = true
		}
		if !progress {
			return lastErr
		}
	}

	_, err := r.client.DeleteBuildPipeline(ctx, devops.DeleteBuildPipelineRequest{BuildPipelineId: r.ID})
	return err
}

func (r *BuildPipeline) Properties() types.Properties {
	return types.NewPropertiesFromStruct(r)
}

func (r *BuildPipeline) String() string {
	if r.Name != nil {
		return *r.Name
	}
	return *r.ID
}
