package resources

import (
	"context"

	"github.com/ekristen/libnuke/pkg/registry"
	"github.com/ekristen/libnuke/pkg/resource"
	"github.com/ekristen/libnuke/pkg/types"
	"github.com/oracle/oci-go-sdk/v65/core"

	"github.com/entigolabs/oci-nuke/pkg/nuke"
)

const NatGatewayResource = "OCINatGateway"

func init() {
	registry.Register(&registry.Registration{
		Name:      NatGatewayResource,
		Scope:     nuke.Compartment,
		Resource:  &NatGateway{},
		Lister:    &NatGatewayLister{},
		DependsOn: []string{RouteTableResource},
	})
}

type NatGatewayLister struct{}

func (l *NatGatewayLister) List(ctx context.Context, o interface{}) ([]resource.Resource, error) {
	opts := o.(*nuke.ListerOpts)
	client, err := virtualNetworkClient(opts)
	if err != nil {
		return nil, err
	}

	var resources []resource.Resource
	page := ""
	for {
		resp, err := client.ListNatGateways(ctx, core.ListNatGatewaysRequest{
			CompartmentId: &opts.CompartmentID,
			Page:          strPtrOrNil(page),
		})
		if err != nil {
			return nil, err
		}
		for _, nat := range resp.Items {
			if nat.LifecycleState == core.NatGatewayLifecycleStateTerminated || nat.LifecycleState == core.NatGatewayLifecycleStateTerminating {
				continue
			}
			resources = append(resources, &NatGateway{client: client, ID: nat.Id, Name: nat.DisplayName})
		}
		if resp.OpcNextPage == nil {
			break
		}
		page = *resp.OpcNextPage
	}
	return resources, nil
}

type NatGateway struct {
	client core.VirtualNetworkClient
	ID     *string
	Name   *string
}

func (r *NatGateway) Remove(ctx context.Context) error {
	_, err := r.client.DeleteNatGateway(ctx, core.DeleteNatGatewayRequest{NatGatewayId: r.ID})
	return err
}

func (r *NatGateway) Properties() types.Properties {
	return types.NewPropertiesFromStruct(r)
}

func (r *NatGateway) String() string {
	if r.Name != nil {
		return *r.Name
	}
	return *r.ID
}
