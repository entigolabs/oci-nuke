package resources

import (
	"context"

	"github.com/ekristen/libnuke/pkg/registry"
	"github.com/ekristen/libnuke/pkg/resource"
	"github.com/ekristen/libnuke/pkg/types"
	"github.com/oracle/oci-go-sdk/v65/core"

	"github.com/entigolabs/oci-nuke/pkg/nuke"
)

const InternetGatewayResource = "OCIInternetGateway"

func init() {
	registry.Register(&registry.Registration{
		Name:     InternetGatewayResource,
		Scope:    nuke.Compartment,
		Resource: &InternetGateway{},
		Lister:   &InternetGatewayLister{},
		// Route rules pointing at the gateway must be gone before it can be deleted.
		DependsOn: []string{RouteTableResource},
	})
}

type InternetGatewayLister struct{}

func (l *InternetGatewayLister) List(ctx context.Context, o interface{}) ([]resource.Resource, error) {
	opts := o.(*nuke.ListerOpts)
	client, err := virtualNetworkClient(opts)
	if err != nil {
		return nil, err
	}

	var resources []resource.Resource
	page := ""
	for {
		resp, err := client.ListInternetGateways(ctx, core.ListInternetGatewaysRequest{
			CompartmentId: &opts.CompartmentID,
			Page:          strPtrOrNil(page),
		})
		if err != nil {
			return nil, err
		}
		for _, ig := range resp.Items {
			if ig.LifecycleState == core.InternetGatewayLifecycleStateTerminated || ig.LifecycleState == core.InternetGatewayLifecycleStateTerminating {
				continue
			}
			resources = append(resources, &InternetGateway{client: client, ID: ig.Id, Name: ig.DisplayName})
		}
		if resp.OpcNextPage == nil {
			break
		}
		page = *resp.OpcNextPage
	}
	return resources, nil
}

type InternetGateway struct {
	client core.VirtualNetworkClient
	ID     *string
	Name   *string
}

func (r *InternetGateway) Remove(ctx context.Context) error {
	_, err := r.client.DeleteInternetGateway(ctx, core.DeleteInternetGatewayRequest{IgId: r.ID})
	return err
}

func (r *InternetGateway) Properties() types.Properties {
	return types.NewPropertiesFromStruct(r)
}

func (r *InternetGateway) String() string {
	if r.Name != nil {
		return *r.Name
	}
	return *r.ID
}
