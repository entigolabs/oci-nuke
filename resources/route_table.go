package resources

import (
	"context"

	"github.com/ekristen/libnuke/pkg/registry"
	"github.com/ekristen/libnuke/pkg/resource"
	"github.com/ekristen/libnuke/pkg/types"
	"github.com/oracle/oci-go-sdk/v65/core"

	"github.com/entigolabs/oci-nuke/pkg/nuke"
)

const RouteTableResource = "OCIRouteTable"

func init() {
	registry.Register(&registry.Registration{
		Name:     RouteTableResource,
		Scope:    nuke.Compartment,
		Resource: &RouteTable{},
		Lister:   &RouteTableLister{},
		// OCI rejects deleting a route table while a subnet still uses it
		// ("is associated with Subnet that is in use") - the mirror image of the
		// gateway-vs-route-table constraint below. Note: if a rerun's scope ever narrows
		// to just route tables with zero real subnets left, libnuke's Scan/Run mismatch
		// (New vs NewDependency counting) can make the run falsely report "No resource to
		// delete" - seen once already with the Subnet dependency on a 3-item scope. Safe
		// here since a full-compartment run always has independent seed items.
		DependsOn: []string{SubnetResource},
	})
}

type RouteTableLister struct{}

func (l *RouteTableLister) List(ctx context.Context, o interface{}) ([]resource.Resource, error) {
	opts := o.(*nuke.ListerOpts)
	client, err := virtualNetworkClient(opts)
	if err != nil {
		return nil, err
	}

	// A VCN's default route table can't be deleted on its own - OCI only allows it to
	// be cleared and cascades its actual removal into DeleteVcn. Collect default IDs so
	// their Remove() clears route rules instead of trying (and forever failing) to delete.
	defaultIDs := map[string]bool{}
	vcnPage := ""
	for {
		vcnResp, err := client.ListVcns(ctx, core.ListVcnsRequest{
			CompartmentId: &opts.CompartmentID,
			Page:          strPtrOrNil(vcnPage),
		})
		if err != nil {
			return nil, err
		}
		for _, v := range vcnResp.Items {
			if v.DefaultRouteTableId != nil {
				defaultIDs[*v.DefaultRouteTableId] = true
			}
		}
		if vcnResp.OpcNextPage == nil {
			break
		}
		vcnPage = *vcnResp.OpcNextPage
	}

	var resources []resource.Resource
	page := ""
	for {
		resp, err := client.ListRouteTables(ctx, core.ListRouteTablesRequest{
			CompartmentId: &opts.CompartmentID,
			Page:          strPtrOrNil(page),
		})
		if err != nil {
			return nil, err
		}
		for _, rt := range resp.Items {
			if rt.LifecycleState == core.RouteTableLifecycleStateTerminated || rt.LifecycleState == core.RouteTableLifecycleStateTerminating {
				continue
			}
			isDefault := rt.Id != nil && defaultIDs[*rt.Id]
			// A default route table's Remove() clears its rules instead of deleting it, so
			// it keeps being listed forever afterwards - and libnuke only marks an item
			// finished once the lister stops returning it (HandleWait re-lists to check).
			// Left listed, it sits in "waiting for removal" until max-wait-retries kills
			// the run, taking the gateways and VCN that depend on it down with it. Once the
			// rules are gone there is genuinely nothing left to do here: the empty table is
			// removed by DeleteVcn.
			if isDefault && len(rt.RouteRules) == 0 {
				continue
			}
			resources = append(resources, &RouteTable{
				client: client, ID: rt.Id, Name: rt.DisplayName, IsDefault: isDefault,
			})
		}
		if resp.OpcNextPage == nil {
			break
		}
		page = *resp.OpcNextPage
	}
	return resources, nil
}

type RouteTable struct {
	client    core.VirtualNetworkClient
	ID        *string
	Name      *string
	IsDefault bool
}

// Non-default route tables are deleted outright. A VCN's default route table can only
// be cleared here - OCI rejects DeleteRouteTable on it while the VCN still exists - and
// clearing its rules is exactly what unblocks the gateways it references; the empty
// table itself is cleaned up automatically when the VCN is deleted afterward.
func (r *RouteTable) Remove(ctx context.Context) error {
	if r.IsDefault {
		_, err := r.client.UpdateRouteTable(ctx, core.UpdateRouteTableRequest{
			RtId: r.ID,
			UpdateRouteTableDetails: core.UpdateRouteTableDetails{
				RouteRules: []core.RouteRule{},
			},
		})
		return err
	}
	_, err := r.client.DeleteRouteTable(ctx, core.DeleteRouteTableRequest{RtId: r.ID})
	return err
}

func (r *RouteTable) Properties() types.Properties {
	return types.NewPropertiesFromStruct(r)
}

func (r *RouteTable) String() string {
	if r.Name != nil {
		return *r.Name
	}
	return *r.ID
}
