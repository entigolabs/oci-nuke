package resources

import (
	"context"
	"fmt"

	liberrors "github.com/ekristen/libnuke/pkg/errors"
	"github.com/ekristen/libnuke/pkg/registry"
	"github.com/ekristen/libnuke/pkg/resource"
	"github.com/ekristen/libnuke/pkg/types"
	"github.com/oracle/oci-go-sdk/v65/common"
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

// DeleteNodePool only accepts the request. HandleWait keeps the node pool waiting until it is
// gone, so the subnets that depend on it are not removed while it still holds VNICs in them.
func (r *OkeNodePool) Remove(ctx context.Context) error {
	_, err := r.client.DeleteNodePool(ctx, containerengine.DeleteNodePoolRequest{NodePoolId: r.ID})
	return err
}

func (r *OkeNodePool) HandleWait(ctx context.Context) error {
	resp, err := r.client.GetNodePool(ctx, containerengine.GetNodePoolRequest{NodePoolId: r.ID})
	if err != nil {
		if svcErr, ok := common.IsServiceError(err); ok && svcErr.GetHTTPStatusCode() == 404 {
			return nil
		}
		return err
	}
	if resp.LifecycleState == containerengine.NodePoolLifecycleStateDeleted {
		return nil
	}
	return liberrors.ErrWaitResource(fmt.Sprintf("node pool is %s", resp.LifecycleState))
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
