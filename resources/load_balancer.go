package resources

import (
	"context"
	"fmt"
	"time"

	"github.com/ekristen/libnuke/pkg/registry"
	"github.com/ekristen/libnuke/pkg/resource"
	"github.com/ekristen/libnuke/pkg/types"
	"github.com/oracle/oci-go-sdk/v65/common"
	"github.com/oracle/oci-go-sdk/v65/loadbalancer"

	"github.com/entigolabs/oci-nuke/pkg/nuke"
)

const LoadBalancerResource = "OCILoadBalancer"

// Load balancers are created out-of-band by in-cluster controllers - the OCI Cloud
// Controller Manager for Service type=LoadBalancer (e.g. ingress-nginx) and the native
// ingress controller - so deleting the OKE cluster orphans them; terraform never knew
// about them either. Without this type a nuke leaves the LB (and its hourly cost)
// behind, and the subnet/NSG deletes 409 on the LB's VNICs/attachments.
func init() {
	registry.Register(&registry.Registration{
		Name:     LoadBalancerResource,
		Scope:    nuke.Compartment,
		Resource: &LoadBalancer{},
		Lister:   &LoadBalancerLister{},
	})
}

type LoadBalancerLister struct{}

func (l *LoadBalancerLister) List(ctx context.Context, o interface{}) ([]resource.Resource, error) {
	opts := o.(*nuke.ListerOpts)
	client, err := loadBalancerClient(opts)
	if err != nil {
		return nil, err
	}

	var resources []resource.Resource
	page := ""
	for {
		resp, err := client.ListLoadBalancers(ctx, loadbalancer.ListLoadBalancersRequest{
			CompartmentId: &opts.CompartmentID,
			Page:          strPtrOrNil(page),
		})
		if err != nil {
			return nil, err
		}
		for _, lb := range resp.Items {
			switch lb.LifecycleState {
			case loadbalancer.LoadBalancerLifecycleStateDeleted, loadbalancer.LoadBalancerLifecycleStateDeleting:
				continue
			}
			resources = append(resources, &LoadBalancer{client: client, ID: lb.Id, Name: lb.DisplayName})
		}
		if resp.OpcNextPage == nil {
			break
		}
		page = *resp.OpcNextPage
	}
	return resources, nil
}

type LoadBalancer struct {
	client loadbalancer.LoadBalancerClient
	ID     *string
	Name   *string
}

// DeleteLoadBalancer only accepts a work request - releasing the LB's VNICs from the
// subnet and its NSG attachments takes longer. Same reason as OkeCluster.Remove: return
// early and libnuke moves on to the subnet/NSG while the VNIC is still attached (409).
func (r *LoadBalancer) Remove(ctx context.Context) error {
	if _, err := r.client.DeleteLoadBalancer(ctx, loadbalancer.DeleteLoadBalancerRequest{LoadBalancerId: r.ID}); err != nil {
		return err
	}

	deadline := time.Now().Add(10 * time.Minute)
	for time.Now().Before(deadline) {
		resp, err := r.client.GetLoadBalancer(ctx, loadbalancer.GetLoadBalancerRequest{LoadBalancerId: r.ID})
		if err != nil {
			if svcErr, ok := common.IsServiceError(err); ok && svcErr.GetHTTPStatusCode() == 404 {
				return nil
			}
			return err
		}
		if resp.LifecycleState == loadbalancer.LoadBalancerLifecycleStateDeleted {
			return nil
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(10 * time.Second):
		}
	}
	return fmt.Errorf("load balancer %s did not reach Deleted state within timeout", *r.ID)
}

func (r *LoadBalancer) Properties() types.Properties {
	return types.NewPropertiesFromStruct(r)
}

func (r *LoadBalancer) String() string {
	if r.Name != nil {
		return *r.Name
	}
	return *r.ID
}
