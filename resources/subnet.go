package resources

import (
	"context"

	"github.com/ekristen/libnuke/pkg/registry"
	"github.com/ekristen/libnuke/pkg/resource"
	"github.com/ekristen/libnuke/pkg/types"
	"github.com/oracle/oci-go-sdk/v65/core"

	"github.com/entigolabs/oci-nuke/pkg/nuke"
)

const SubnetResource = "OCISubnet"

func init() {
	registry.Register(&registry.Registration{
		Name:     SubnetResource,
		Scope:    nuke.Compartment,
		Resource: &Subnet{},
		Lister:   &SubnetLister{},
	})
}

type SubnetLister struct{}

func (l *SubnetLister) List(ctx context.Context, o interface{}) ([]resource.Resource, error) {
	opts := o.(*nuke.ListerOpts)
	client, err := virtualNetworkClient(opts)
	if err != nil {
		return nil, err
	}

	var resources []resource.Resource
	page := ""
	for {
		resp, err := client.ListSubnets(ctx, core.ListSubnetsRequest{
			CompartmentId: &opts.CompartmentID,
			Page:          strPtrOrNil(page),
		})
		if err != nil {
			return nil, err
		}
		for _, s := range resp.Items {
			if s.LifecycleState == core.SubnetLifecycleStateTerminated || s.LifecycleState == core.SubnetLifecycleStateTerminating {
				continue
			}
			resources = append(resources, &Subnet{client: client, ID: s.Id, Name: s.DisplayName})
		}
		if resp.OpcNextPage == nil {
			break
		}
		page = *resp.OpcNextPage
	}
	return resources, nil
}

type Subnet struct {
	client core.VirtualNetworkClient
	ID     *string
	Name   *string
}

func (r *Subnet) Remove(ctx context.Context) error {
	_, err := r.client.DeleteSubnet(ctx, core.DeleteSubnetRequest{SubnetId: r.ID})
	return err
}

func (r *Subnet) Properties() types.Properties {
	return types.NewPropertiesFromStruct(r)
}

func (r *Subnet) String() string {
	if r.Name != nil {
		return *r.Name
	}
	return *r.ID
}
