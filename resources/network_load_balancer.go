package resources

import (
	"context"
	"fmt"
	"time"

	"github.com/ekristen/libnuke/pkg/registry"
	"github.com/ekristen/libnuke/pkg/resource"
	"github.com/ekristen/libnuke/pkg/types"
	"github.com/oracle/oci-go-sdk/v65/common"
	"github.com/oracle/oci-go-sdk/v65/networkloadbalancer"

	"github.com/entigolabs/oci-nuke/pkg/nuke"
)

const NetworkLoadBalancerResource = "OCINetworkLoadBalancer"

// A Service of type=LoadBalancer asking for a UDP listener (oci.oraclecloud.com/load-
// balancer-type: "nlb", e.g. wireguard) gets an NLB instead of a classic LoadBalancer -
// same out-of-band-creation story as LoadBalancerResource, so it needs the same
// standalone resource type or it's left behind (and its subnet/NSG deletes 409).
func init() {
	registry.Register(&registry.Registration{
		Name:     NetworkLoadBalancerResource,
		Scope:    nuke.Compartment,
		Resource: &NetworkLoadBalancer{},
		Lister:   &NetworkLoadBalancerLister{},
	})
}

type NetworkLoadBalancerLister struct{}

func (l *NetworkLoadBalancerLister) List(ctx context.Context, o interface{}) ([]resource.Resource, error) {
	opts := o.(*nuke.ListerOpts)
	client, err := networkLoadBalancerClient(opts)
	if err != nil {
		return nil, err
	}

	var resources []resource.Resource
	page := ""
	for {
		resp, err := client.ListNetworkLoadBalancers(ctx, networkloadbalancer.ListNetworkLoadBalancersRequest{
			CompartmentId: &opts.CompartmentID,
			Page:          strPtrOrNil(page),
		})
		if err != nil {
			return nil, err
		}
		for _, nlb := range resp.Items {
			switch nlb.LifecycleState {
			case networkloadbalancer.LifecycleStateDeleted, networkloadbalancer.LifecycleStateDeleting:
				continue
			}
			resources = append(resources, &NetworkLoadBalancer{client: client, ID: nlb.Id, Name: nlb.DisplayName})
		}
		if resp.OpcNextPage == nil {
			break
		}
		page = *resp.OpcNextPage
	}
	return resources, nil
}

type NetworkLoadBalancer struct {
	client networkloadbalancer.NetworkLoadBalancerClient
	ID     *string
	Name   *string
}

// DeleteNetworkLoadBalancer only accepts a work request - releasing the NLB's VNIC from
// the subnet and its NSG attachments takes longer. Same reason as LoadBalancer.Remove:
// return early and libnuke moves on to the subnet/NSG while the VNIC is still attached.
func (r *NetworkLoadBalancer) Remove(ctx context.Context) error {
	if _, err := r.client.DeleteNetworkLoadBalancer(ctx, networkloadbalancer.DeleteNetworkLoadBalancerRequest{NetworkLoadBalancerId: r.ID}); err != nil {
		return err
	}

	deadline := time.Now().Add(10 * time.Minute)
	for time.Now().Before(deadline) {
		resp, err := r.client.GetNetworkLoadBalancer(ctx, networkloadbalancer.GetNetworkLoadBalancerRequest{NetworkLoadBalancerId: r.ID})
		if err != nil {
			if svcErr, ok := common.IsServiceError(err); ok && svcErr.GetHTTPStatusCode() == 404 {
				return nil
			}
			return err
		}
		if resp.LifecycleState == networkloadbalancer.LifecycleStateDeleted {
			return nil
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(10 * time.Second):
		}
	}
	return fmt.Errorf("network load balancer %s did not reach Deleted state within timeout", *r.ID)
}

func (r *NetworkLoadBalancer) Properties() types.Properties {
	return types.NewPropertiesFromStruct(r)
}

func (r *NetworkLoadBalancer) String() string {
	if r.Name != nil {
		return *r.Name
	}
	return *r.ID
}
