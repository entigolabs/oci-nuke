package resources

import (
	"context"
	"fmt"
	"time"

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

// DeleteNodePool only accepts the request - the underlying compute instances take
// several minutes to actually terminate and release their VNICs. Returning as soon as
// the delete call is accepted (rather than once the pool is actually gone) makes libnuke
// think the dependency is satisfied and move on to the subnet, which then 409s with
// "references the VNIC ..." since the node's VNIC hasn't been released yet.
func (r *OkeNodePool) Remove(ctx context.Context) error {
	if _, err := r.client.DeleteNodePool(ctx, containerengine.DeleteNodePoolRequest{NodePoolId: r.ID}); err != nil {
		return err
	}

	deadline := time.Now().Add(10 * time.Minute)
	for time.Now().Before(deadline) {
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
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(10 * time.Second):
		}
	}
	return fmt.Errorf("node pool %s did not reach Deleted state within timeout", *r.ID)
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
