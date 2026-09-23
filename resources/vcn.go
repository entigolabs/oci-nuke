package resources

import (
	"context"

	"github.com/ekristen/libnuke/pkg/registry"
	"github.com/ekristen/libnuke/pkg/resource"
	"github.com/ekristen/libnuke/pkg/types"
	"github.com/oracle/oci-go-sdk/v65/core"

	"github.com/entigolabs/oci-nuke/pkg/nuke"
)

const VcnResource = "OCIVcn"

func init() {
	registry.Register(&registry.Registration{
		Name:     VcnResource,
		Scope:    nuke.Compartment,
		Resource: &Vcn{},
		Lister:   &VcnLister{},
		// A private DNS zone lives in a view, and the view a deployment's zones go into
		// is the default view of this VCN's own resolver. The zone outlives the VCN
		// either way - that is how they piled up - but deleting it while its view is
		// still there is the path the DNS API is built for, so the VCN waits.
		DependsOn: []string{
			DnsZoneResource,
			SubnetResource,
			RouteTableResource,
			InternetGatewayResource,
			NatGatewayResource,
			ServiceGatewayResource,
			NetworkSecurityGroupResource,
		},
	})
}

type VcnLister struct{}

func (l *VcnLister) List(ctx context.Context, o interface{}) ([]resource.Resource, error) {
	opts := o.(*nuke.ListerOpts)
	client, err := virtualNetworkClient(opts)
	if err != nil {
		return nil, err
	}

	var resources []resource.Resource
	page := ""
	for {
		resp, err := client.ListVcns(ctx, core.ListVcnsRequest{
			CompartmentId: &opts.CompartmentID,
			Page:          strPtrOrNil(page),
		})
		if err != nil {
			return nil, err
		}
		for _, v := range resp.Items {
			if v.LifecycleState == core.VcnLifecycleStateTerminated || v.LifecycleState == core.VcnLifecycleStateTerminating {
				continue
			}
			resources = append(resources, &Vcn{client: client, ID: v.Id, Name: v.DisplayName})
		}
		if resp.OpcNextPage == nil {
			break
		}
		page = *resp.OpcNextPage
	}
	return resources, nil
}

type Vcn struct {
	client core.VirtualNetworkClient
	ID     *string
	Name   *string
}

func (r *Vcn) Remove(ctx context.Context) error {
	_, err := r.client.DeleteVcn(ctx, core.DeleteVcnRequest{VcnId: r.ID})
	return err
}

func (r *Vcn) Properties() types.Properties {
	return types.NewPropertiesFromStruct(r)
}

func (r *Vcn) String() string {
	if r.Name != nil {
		return *r.Name
	}
	return *r.ID
}
