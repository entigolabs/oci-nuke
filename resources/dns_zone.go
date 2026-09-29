package resources

import (
	"context"
	"errors"
	"net/http"
	"strings"

	"github.com/ekristen/libnuke/pkg/registry"
	"github.com/ekristen/libnuke/pkg/resource"
	"github.com/ekristen/libnuke/pkg/types"
	"github.com/oracle/oci-go-sdk/v65/common"
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
					client:        client,
					compartmentID: opts.CompartmentID,
					ID:            z.Id,
					Name:          z.Name,
					Scope:         string(z.Scope),
					ViewID:        z.ViewId,
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
	// Unexported so that it stays out of Properties; finding the parent zone needs it.
	compartmentID string
	ID            *string
	Name          *string
	Scope         string
	ViewID        *string
}

// Addressed by OCID, not by name: a private zone's name is unique only within its view,
// and the API demands the view alongside the name to disambiguate. The scope still has to
// be stated - an unscoped request is a GLOBAL one, and would miss a private zone entirely.
//
// A public zone delegated from a parent zone in the same compartment takes its NS records in
// the parent with it. Otherwise a kept parent, such as a shared test domain, collects a
// delegation per torn-down zone, and recreating the zone fails on the one already there.
func (r *DnsZone) Remove(ctx context.Context) error {
	if r.Scope == string(dns.ListZonesScopeGlobal) && r.Name != nil {
		if err := deleteDelegation(ctx, r.client, r.compartmentID, *r.Name); err != nil {
			return err
		}
	}
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

// dnsDelegationAPI is the part of the DNS client that deleteDelegation uses.
type dnsDelegationAPI interface {
	ListZones(context.Context, dns.ListZonesRequest) (dns.ListZonesResponse, error)
	DeleteRRSet(context.Context, dns.DeleteRRSetRequest) (dns.DeleteRRSetResponse, error)
}

// deleteDelegation removes the NS records for zoneName from the nearest enclosing zone in the
// compartment. A missing parent or record is not an error.
func deleteDelegation(ctx context.Context, api dnsDelegationAPI, compartmentID, zoneName string) error {
	for _, parent := range parentZoneNames(zoneName) {
		resp, err := api.ListZones(ctx, dns.ListZonesRequest{
			CompartmentId: &compartmentID,
			Name:          &parent,
			Scope:         dns.ListZonesScopeGlobal,
		})
		if err != nil {
			return err
		}
		for _, z := range resp.Items {
			switch z.LifecycleState {
			case dns.ZoneSummaryLifecycleStateDeleted, dns.ZoneSummaryLifecycleStateDeleting:
				continue
			}
			_, err := api.DeleteRRSet(ctx, dns.DeleteRRSetRequest{
				ZoneNameOrId:  z.Id,
				Domain:        &zoneName,
				Rtype:         common.String("NS"),
				CompartmentId: &compartmentID,
				Scope:         dns.DeleteRRSetScopeGlobal,
			})
			if err != nil && !isNotFound(err) {
				return err
			}
			return nil
		}
	}
	return nil
}

// parentZoneNames lists the names a parent zone of name could have, nearest first, down to
// two labels: a.b.example.com gives b.example.com, then example.com.
func parentZoneNames(name string) []string {
	labels := strings.Split(strings.TrimSuffix(name, "."), ".")
	var names []string
	for i := 1; len(labels)-i >= 2; i++ {
		names = append(names, strings.Join(labels[i:], "."))
	}
	return names
}

func isNotFound(err error) bool {
	var svcErr common.ServiceError
	return errors.As(err, &svcErr) && svcErr.GetHTTPStatusCode() == http.StatusNotFound
}
