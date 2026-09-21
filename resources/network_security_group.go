package resources

import (
	"context"

	"github.com/ekristen/libnuke/pkg/registry"
	"github.com/ekristen/libnuke/pkg/resource"
	"github.com/ekristen/libnuke/pkg/types"
	"github.com/oracle/oci-go-sdk/v65/core"

	"github.com/entigolabs/oci-nuke/pkg/nuke"
)

const NetworkSecurityGroupResource = "OCINetworkSecurityGroup"

func init() {
	registry.Register(&registry.Registration{
		Name:     NetworkSecurityGroupResource,
		Scope:    nuke.Compartment,
		Resource: &NetworkSecurityGroup{},
		Lister:   &NetworkSecurityGroupLister{},
		// A VNIC keeps its NSG membership until the VNIC itself is gone, so this can't be
		// removed while an OKE cluster endpoint, node pool, or load balancer (attached to
		// NSGs via the IngressClass/Service annotations) still references it.
		DependsOn: []string{OkeClusterResource, OkeNodePoolResource, LoadBalancerResource, NetworkLoadBalancerResource},
	})
}

type NetworkSecurityGroupLister struct{}

func (l *NetworkSecurityGroupLister) List(ctx context.Context, o interface{}) ([]resource.Resource, error) {
	opts := o.(*nuke.ListerOpts)
	client, err := virtualNetworkClient(opts)
	if err != nil {
		return nil, err
	}

	var resources []resource.Resource
	page := ""
	for {
		resp, err := client.ListNetworkSecurityGroups(ctx, core.ListNetworkSecurityGroupsRequest{
			CompartmentId: &opts.CompartmentID,
			Page:          strPtrOrNil(page),
		})
		if err != nil {
			return nil, err
		}
		for _, nsg := range resp.Items {
			if nsg.LifecycleState == core.NetworkSecurityGroupLifecycleStateTerminated || nsg.LifecycleState == core.NetworkSecurityGroupLifecycleStateTerminating {
				continue
			}
			resources = append(resources, &NetworkSecurityGroup{client: client, ID: nsg.Id, Name: nsg.DisplayName})
		}
		if resp.OpcNextPage == nil {
			break
		}
		page = *resp.OpcNextPage
	}
	return resources, nil
}

type NetworkSecurityGroup struct {
	client core.VirtualNetworkClient
	ID     *string
	Name   *string
}

// Its security rules have no separate lifecycle - they're deleted along with the group.
func (r *NetworkSecurityGroup) Remove(ctx context.Context) error {
	_, err := r.client.DeleteNetworkSecurityGroup(ctx, core.DeleteNetworkSecurityGroupRequest{NetworkSecurityGroupId: r.ID})
	return err
}

func (r *NetworkSecurityGroup) Properties() types.Properties {
	return types.NewPropertiesFromStruct(r)
}

func (r *NetworkSecurityGroup) String() string {
	if r.Name != nil {
		return *r.Name
	}
	return *r.ID
}
