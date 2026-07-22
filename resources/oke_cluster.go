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

const OkeClusterResource = "OCIOkeCluster"

func init() {
	registry.Register(&registry.Registration{
		Name:      OkeClusterResource,
		Scope:     nuke.Compartment,
		Resource:  &OkeCluster{},
		Lister:    &OkeClusterLister{},
		DependsOn: []string{OkeNodePoolResource},
	})
}

type OkeClusterLister struct{}

func (l *OkeClusterLister) List(ctx context.Context, o interface{}) ([]resource.Resource, error) {
	opts := o.(*nuke.ListerOpts)
	client, err := containerEngineClient(opts)
	if err != nil {
		return nil, err
	}

	var resources []resource.Resource
	page := ""
	for {
		resp, err := client.ListClusters(ctx, containerengine.ListClustersRequest{
			CompartmentId: &opts.CompartmentID,
			Page:          strPtrOrNil(page),
		})
		if err != nil {
			return nil, err
		}
		for _, c := range resp.Items {
			switch c.LifecycleState {
			case containerengine.ClusterLifecycleStateDeleted, containerengine.ClusterLifecycleStateDeleting:
				continue
			}
			resources = append(resources, &OkeCluster{client: client, ID: c.Id, Name: c.Name})
		}
		if resp.OpcNextPage == nil {
			break
		}
		page = *resp.OpcNextPage
	}
	return resources, nil
}

type OkeCluster struct {
	client containerengine.ContainerEngineClient
	ID     *string
	Name   *string
}

// DeleteCluster only accepts the request - releasing the endpoint's service VNIC from
// its subnet takes a little while longer. Returning early makes libnuke think the
// dependency is satisfied and move on to the subnet, which then 409s with "references
// the service VNIC ...".
func (r *OkeCluster) Remove(ctx context.Context) error {
	if _, err := r.client.DeleteCluster(ctx, containerengine.DeleteClusterRequest{ClusterId: r.ID}); err != nil {
		return err
	}

	deadline := time.Now().Add(10 * time.Minute)
	for time.Now().Before(deadline) {
		resp, err := r.client.GetCluster(ctx, containerengine.GetClusterRequest{ClusterId: r.ID})
		if err != nil {
			if svcErr, ok := common.IsServiceError(err); ok && svcErr.GetHTTPStatusCode() == 404 {
				return nil
			}
			return err
		}
		if resp.LifecycleState == containerengine.ClusterLifecycleStateDeleted {
			return nil
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(10 * time.Second):
		}
	}
	return fmt.Errorf("cluster %s did not reach Deleted state within timeout", *r.ID)
}

func (r *OkeCluster) Properties() types.Properties {
	return types.NewPropertiesFromStruct(r)
}

func (r *OkeCluster) String() string {
	if r.Name != nil {
		return *r.Name
	}
	return *r.ID
}
