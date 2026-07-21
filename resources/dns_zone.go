package resources

import (
	"context"

	"github.com/ekristen/libnuke/pkg/registry"
	"github.com/ekristen/libnuke/pkg/resource"
	"github.com/ekristen/libnuke/pkg/types"
	"github.com/oracle/oci-go-sdk/v65/dns"

	"github.com/entigolabs/oci-nuke/pkg/nuke"
)

const DnsZoneResource = "OCIDnsZone"

func init() {
	registry.Register(&registry.Registration{
		Name:     DnsZoneResource,
		Scope:    nuke.Compartment,
		Resource: &DnsZone{},
		Lister:   &DnsZoneLister{},
	})
}

type DnsZoneLister struct{}

func (l *DnsZoneLister) List(ctx context.Context, o interface{}) ([]resource.Resource, error) {
	opts := o.(*nuke.ListerOpts)
	client, err := dnsClient(opts)
	if err != nil {
		return nil, err
	}

	var resources []resource.Resource
	page := ""
	for {
		resp, err := client.ListZones(ctx, dns.ListZonesRequest{
			CompartmentId: &opts.CompartmentID,
			Page:          strPtrOrNil(page),
		})
		if err != nil {
			return nil, err
		}
		for _, z := range resp.Items {
			switch z.LifecycleState {
			case dns.ZoneSummaryLifecycleStateDeleted, dns.ZoneSummaryLifecycleStateDeleting:
				continue
			}
			resources = append(resources, &DnsZone{client: client, Name: z.Name})
		}
		if resp.OpcNextPage == nil {
			break
		}
		page = *resp.OpcNextPage
	}
	return resources, nil
}

type DnsZone struct {
	client dns.DnsClient
	Name   *string
}

func (r *DnsZone) Remove(ctx context.Context) error {
	_, err := r.client.DeleteZone(ctx, dns.DeleteZoneRequest{ZoneNameOrId: r.Name})
	return err
}

func (r *DnsZone) Properties() types.Properties {
	return types.NewPropertiesFromStruct(r)
}

func (r *DnsZone) String() string {
	return *r.Name
}
