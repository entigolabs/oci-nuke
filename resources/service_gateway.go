package resources

import (
	"context"

	"github.com/ekristen/libnuke/pkg/registry"
	"github.com/ekristen/libnuke/pkg/resource"
	"github.com/ekristen/libnuke/pkg/types"
	"github.com/oracle/oci-go-sdk/v65/core"

	"github.com/entigolabs/oci-nuke/pkg/nuke"
)

const ServiceGatewayResource = "OCIServiceGateway"

func init() {
	registry.Register(&registry.Registration{
		Name:      ServiceGatewayResource,
		Scope:     nuke.Compartment,
		Resource:  &ServiceGateway{},
		Lister:    &ServiceGatewayLister{},
		DependsOn: []string{RouteTableResource},
	})
}

type ServiceGatewayLister struct{}

func (l *ServiceGatewayLister) List(ctx context.Context, o interface{}) ([]resource.Resource, error) {
	opts := o.(*nuke.ListerOpts)
	client, err := virtualNetworkClient(opts)
	if err != nil {
		return nil, err
	}

	var resources []resource.Resource
	page := ""
	for {
		resp, err := client.ListServiceGateways(ctx, core.ListServiceGatewaysRequest{
			CompartmentId: &opts.CompartmentID,
			Page:          strPtrOrNil(page),
		})
		if err != nil {
			return nil, err
		}
		for _, sgw := range resp.Items {
			if sgw.LifecycleState == core.ServiceGatewayLifecycleStateTerminated || sgw.LifecycleState == core.ServiceGatewayLifecycleStateTerminating {
				continue
			}
			resources = append(resources, &ServiceGateway{client: client, ID: sgw.Id, Name: sgw.DisplayName})
		}
		if resp.OpcNextPage == nil {
			break
		}
		page = *resp.OpcNextPage
	}
	return resources, nil
}

type ServiceGateway struct {
	client core.VirtualNetworkClient
	ID     *string
	Name   *string
}

func (r *ServiceGateway) Remove(ctx context.Context) error {
	_, err := r.client.DeleteServiceGateway(ctx, core.DeleteServiceGatewayRequest{ServiceGatewayId: r.ID})
	return err
}

func (r *ServiceGateway) Properties() types.Properties {
	return types.NewPropertiesFromStruct(r)
}

func (r *ServiceGateway) String() string {
	if r.Name != nil {
		return *r.Name
	}
	return *r.ID
}
