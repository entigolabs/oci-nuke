package resources

import (
	"context"

	"github.com/ekristen/libnuke/pkg/registry"
	"github.com/ekristen/libnuke/pkg/resource"
	"github.com/ekristen/libnuke/pkg/types"
	"github.com/oracle/oci-go-sdk/v65/containerengine"

	"github.com/entigolabs/oci-nuke/pkg/nuke"
)

const OkeNodePoolResource = "OCIOkeNodePool"

func init() {
	registry.Register(&registry.Registration{
		Name:     OkeNodePoolResource,
		Scope:    nuke.Compartment,
		Resource: &OkeNodePool{},
		Lister:   &OkeNodePoolLister{},
	})
}

type OkeNodePoolLister struct{}

func (l *OkeNodePoolLister) List(ctx context.Context, o interface{}) ([]resource.Resource, error) {
	opts := o.(*nuke.ListerOpts)
	client, err := containerEngineClient(opts)
	if err != nil {
		return nil, err
	}

	var resources []resource.Resource
	page := ""
	for {
		resp, err := client.ListNodePools(ctx, containerengine.ListNodePoolsRequest{
			CompartmentId: &opts.CompartmentID,
			Page:          strPtrOrNil(page),
		})
		if err != nil {
			return nil, err
		}
		for _, np := range resp.Items {
			switch np.LifecycleState {
			case containerengine.NodePoolLifecycleStateDeleted, containerengine.NodePoolLifecycleStateDeleting:
				continue
			}
			resources = append(resources, &OkeNodePool{client: client, ID: np.Id, Name: np.Name})
		}
		if resp.OpcNextPage == nil {
			break
		}
		page = *resp.OpcNextPage
	}
	return resources, nil
}

type OkeNodePool struct {
	client containerengine.ContainerEngineClient
	ID     *string
	Name   *string
}

func (r *OkeNodePool) Remove(ctx context.Context) error {
	_, err := r.client.DeleteNodePool(ctx, containerengine.DeleteNodePoolRequest{NodePoolId: r.ID})
	return err
}

func (r *OkeNodePool) Properties() types.Properties {
	return types.NewPropertiesFromStruct(r)
}

func (r *OkeNodePool) String() string {
	if r.Name != nil {
		return *r.Name
	}
	return *r.ID
}
