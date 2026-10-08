package resources

import (
	"context"
	"net/http"

	"github.com/ekristen/libnuke/pkg/registry"
	"github.com/ekristen/libnuke/pkg/resource"
	"github.com/ekristen/libnuke/pkg/types"
	"github.com/oracle/oci-go-sdk/v65/common"
	"github.com/oracle/oci-go-sdk/v65/devops"

	"github.com/entigolabs/oci-nuke/pkg/nuke"
)

const BuildRunResource = "OCIBuildRun"

func init() {
	registry.Register(&registry.Registration{
		Name:     BuildRunResource,
		Scope:    nuke.Compartment,
		Resource: &BuildRun{},
		Lister:   &BuildRunLister{},
	})
}

type BuildRunLister struct{}

func (l *BuildRunLister) List(ctx context.Context, o interface{}) ([]resource.Resource, error) {
	opts := o.(*nuke.ListerOpts)
	client, err := devopsClient(opts)
	if err != nil {
		return nil, err
	}

	var resources []resource.Resource
	page := ""
	for {
		resp, err := client.ListBuildRuns(ctx, devops.ListBuildRunsRequest{
			CompartmentId: &opts.CompartmentID,
			Page:          strPtrOrNil(page),
		})
		if err != nil {
			return nil, err
		}
		for _, run := range resp.Items {
			if run.LifecycleState == devops.BuildRunLifecycleStateDeleting {
				continue
			}
			resources = append(resources, &BuildRun{client: client, ID: run.Id, Name: run.DisplayName})
		}
		if resp.OpcNextPage == nil {
			break
		}
		page = *resp.OpcNextPage
	}
	return resources, nil
}

type BuildRun struct {
	client devops.DevopsClient
	ID     *string
	Name   *string
}

func (r *BuildRun) Remove(ctx context.Context) error {
	req, err := devops.GetBuildRunRequest{BuildRunId: r.ID}.HTTPRequest(http.MethodDelete, "/buildRuns/{buildRunId}", nil, nil)
	if err != nil {
		return err
	}
	resp, err := r.client.Call(ctx, &req)
	common.CloseBodyIfValid(resp)
	return err
}

func (r *BuildRun) Properties() types.Properties {
	return types.NewPropertiesFromStruct(r)
}

func (r *BuildRun) String() string {
	if r.Name != nil {
		return *r.Name
	}
	return *r.ID
}
