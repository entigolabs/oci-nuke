package resources

import (
	"context"
	"errors"
	"net/http"
	"reflect"
	"testing"

	"github.com/oracle/oci-go-sdk/v65/dns"
)

type fakeDelegationAPI struct {
	zones     map[string]string // zone name -> zone id
	deleteErr error
	deleted   []dns.DeleteRRSetRequest
}

func (f *fakeDelegationAPI) ListZones(_ context.Context, req dns.ListZonesRequest) (dns.ListZonesResponse, error) {
	var resp dns.ListZonesResponse
	if id, ok := f.zones[*req.Name]; ok {
		resp.Items = []dns.ZoneSummary{{Id: &id, Name: req.Name, LifecycleState: dns.ZoneSummaryLifecycleStateActive}}
	}
	return resp, nil
}

func (f *fakeDelegationAPI) DeleteRRSet(_ context.Context, req dns.DeleteRRSetRequest) (dns.DeleteRRSetResponse, error) {
	f.deleted = append(f.deleted, req)
	return dns.DeleteRRSetResponse{}, f.deleteErr
}

type notFoundError struct{}

func (notFoundError) Error() string          { return "404 NotAuthorizedOrNotFound" }
func (notFoundError) GetHTTPStatusCode() int { return http.StatusNotFound }
func (notFoundError) GetMessage() string     { return "not found" }
func (notFoundError) GetCode() string        { return "NotAuthorizedOrNotFound" }
func (notFoundError) GetOpcRequestID() string {
	return ""
}

func TestParentZoneNames(t *testing.T) {
	got := parentZoneNames("biz-net-dns.oci.infralib.entigo.io")
	want := []string{"oci.infralib.entigo.io", "infralib.entigo.io", "entigo.io"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v, want %v", got, want)
	}
	if got := parentZoneNames("entigo.io"); got != nil {
		t.Fatalf("a two-label name has no parent to look for, got %v", got)
	}
}

func TestDeleteDelegationRemovesTheNSRecordsFromTheNearestParent(t *testing.T) {
	api := &fakeDelegationAPI{zones: map[string]string{
		"oci.infralib.entigo.io": "parent-near",
		"entigo.io":              "parent-far",
	}}

	if err := deleteDelegation(context.Background(), api, "compartment", "biz-net-dns.oci.infralib.entigo.io"); err != nil {
		t.Fatal(err)
	}

	if len(api.deleted) != 1 {
		t.Fatalf("got %d deletes, want 1", len(api.deleted))
	}
	req := api.deleted[0]
	if *req.ZoneNameOrId != "parent-near" || *req.Domain != "biz-net-dns.oci.infralib.entigo.io" || *req.Rtype != "NS" {
		t.Fatalf("deleted %s %s from %s, want the NS records of the zone from the nearest parent",
			*req.Rtype, *req.Domain, *req.ZoneNameOrId)
	}
}

func TestDeleteDelegationWithoutParentDeletesNothing(t *testing.T) {
	api := &fakeDelegationAPI{zones: map[string]string{}}

	if err := deleteDelegation(context.Background(), api, "compartment", "biz-net-dns.oci.infralib.entigo.io"); err != nil {
		t.Fatal(err)
	}
	if len(api.deleted) != 0 {
		t.Fatalf("got %d deletes, want none", len(api.deleted))
	}
}

func TestDeleteDelegationToleratesAMissingRecord(t *testing.T) {
	api := &fakeDelegationAPI{zones: map[string]string{"oci.infralib.entigo.io": "parent"}, deleteErr: notFoundError{}}

	if err := deleteDelegation(context.Background(), api, "compartment", "biz-net-dns.oci.infralib.entigo.io"); err != nil {
		t.Fatalf("a missing delegation is not an error, got %v", err)
	}
}

func TestDeleteDelegationReportsOtherErrors(t *testing.T) {
	api := &fakeDelegationAPI{zones: map[string]string{"oci.infralib.entigo.io": "parent"}, deleteErr: errors.New("500 InternalServerError")}

	if err := deleteDelegation(context.Background(), api, "compartment", "biz-net-dns.oci.infralib.entigo.io"); err == nil {
		t.Fatal("want the error, got nil")
	}
}
