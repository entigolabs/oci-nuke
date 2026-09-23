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

// ListZones reports one DNS scope at a time and treats a request that names no scope as
// GLOBAL, so both scopes have to be asked for by name. Private zones are not an edge
// case here: entigo-infralib's oracle/dns module creates one per domain marked private,
// and nothing else in the compartment removes them.
var dnsZoneScopes = []dns.ListZonesScopeEnum{
	dns.ListZonesScopeGlobal,
	dns.ListZonesScopePrivate,
}

type DnsZoneLister struct{}

func (l *DnsZoneLister) List(ctx context.Context, o interface{}) ([]resource.Resource, error) {
	opts := o.(*nuke.ListerOpts)
	client, err := dnsClient(opts)
	if err != nil {
		return nil, err
	}

	var resources []resource.Resource
	for _, scope := range dnsZoneScopes {
		page := ""
		for {
			resp, err := client.ListZones(ctx, dns.ListZonesRequest{
				CompartmentId: &opts.CompartmentID,
				Scope:         scope,
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
				// Some private zones belong to the DNS service rather than to a
				// deployment - a VCN with a DNS label gets <label>.oraclevcn.com in its
				// resolver's default view - and OCI rejects every delete of one. Listed,
				// such a zone would be an item that can never succeed, and two rounds of
				// retries later the whole run exits failed. The VCN that owns it takes it
				// away when it goes.
				if z.IsProtected != nil && *z.IsProtected {
					continue
				}
				resources = append(resources, &DnsZone{
					client: client,
					ID:     z.Id,
					Name:   z.Name,
					Scope:  string(z.Scope),
					ViewID: z.ViewId,
				})
			}
			if resp.OpcNextPage == nil {
				break
			}
			page = *resp.OpcNextPage
		}
	}
	return resources, nil
}

type DnsZone struct {
	client dns.DnsClient
	ID     *string
	Name   *string
	Scope  string
	ViewID *string
}

// Addressed by OCID, not by name: a private zone's name is unique only within its view,
// and the API demands the view alongside the name to disambiguate. The scope still has to
// be stated - an unscoped request is a GLOBAL one, and would miss a private zone entirely.
func (r *DnsZone) Remove(ctx context.Context) error {
	_, err := r.client.DeleteZone(ctx, dns.DeleteZoneRequest{
		ZoneNameOrId: r.ID,
		Scope:        dns.DeleteZoneScopeEnum(r.Scope),
	})
	return err
}

func (r *DnsZone) Properties() types.Properties {
	return types.NewPropertiesFromStruct(r)
}

func (r *DnsZone) String() string {
	if r.Name != nil {
		return *r.Name
	}
	return *r.ID
}
