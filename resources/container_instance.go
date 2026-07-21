package resources

import (
	"context"

	"github.com/ekristen/libnuke/pkg/registry"
	"github.com/ekristen/libnuke/pkg/resource"
	"github.com/ekristen/libnuke/pkg/types"
	"github.com/oracle/oci-go-sdk/v65/containerinstances"

	"github.com/entigolabs/oci-nuke/pkg/nuke"
)

const ContainerInstanceResource = "OCIContainerInstance"

func init() {
	registry.Register(&registry.Registration{
		Name:     ContainerInstanceResource,
		Scope:    nuke.Compartment,
		Resource: &ContainerInstance{},
		Lister:   &ContainerInstanceLister{},
	})
}

type ContainerInstanceLister struct{}

// The agent's job runner Container Instances self-terminate once the terraform/helm
// job completes, so this normally finds nothing - it exists defensively, for a run
// interrupted mid-job.
func (l *ContainerInstanceLister) List(ctx context.Context, o interface{}) ([]resource.Resource, error) {
	opts := o.(*nuke.ListerOpts)
	client, err := containerInstanceClient(opts)
	if err != nil {
		return nil, err
	}

	var resources []resource.Resource
	page := ""
	for {
		resp, err := client.ListContainerInstances(ctx, containerinstances.ListContainerInstancesRequest{
			CompartmentId: &opts.CompartmentID,
			Page:          strPtrOrNil(page),
		})
		if err != nil {
			return nil, err
		}
		for _, ci := range resp.Items {
			switch ci.LifecycleState {
			case containerinstances.ContainerInstanceLifecycleStateDeleted,
				containerinstances.ContainerInstanceLifecycleStateDeleting:
				continue
			}
			resources = append(resources, &ContainerInstance{client: client, ID: ci.Id, Name: ci.DisplayName})
		}
		if resp.OpcNextPage == nil {
			break
		}
		page = *resp.OpcNextPage
	}
	return resources, nil
}

type ContainerInstance struct {
	client containerinstances.ContainerInstanceClient
	ID     *string
	Name   *string
}

func (r *ContainerInstance) Remove(ctx context.Context) error {
	_, err := r.client.DeleteContainerInstance(ctx, containerinstances.DeleteContainerInstanceRequest{ContainerInstanceId: r.ID})
	return err
}

func (r *ContainerInstance) Properties() types.Properties {
	return types.NewPropertiesFromStruct(r)
}

func (r *ContainerInstance) String() string {
	if r.Name != nil {
		return *r.Name
	}
	return *r.ID
}
