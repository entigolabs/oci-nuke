package resources

import (
	"context"
	"time"

	"github.com/ekristen/libnuke/pkg/registry"
	"github.com/ekristen/libnuke/pkg/resource"
	"github.com/ekristen/libnuke/pkg/types"
	"github.com/oracle/oci-go-sdk/v65/certificatesmanagement"
	"github.com/oracle/oci-go-sdk/v65/common"

	"github.com/entigolabs/oci-nuke/pkg/nuke"
)

const CertificateResource = "OCICertificate"

// Certificates-service certificates are created out-of-band by the OCI native ingress
// controller (one "oci-nic-<uuid>" per TLS ingress) and never deleted when the LB or the
// controller goes away - found orphaned after the NIC -> ingress-nginx migration.
// Certificate *associations* block deletion, but those die with the LB listener, and
// OCILoadBalancer runs unordered alongside this; libnuke's failure retries cover the
// window where the LB still holds the association.
func init() {
	registry.Register(&registry.Registration{
		Name:     CertificateResource,
		Scope:    nuke.Compartment,
		Resource: &Certificate{},
		Lister:   &CertificateLister{},
	})
}

type CertificateLister struct{}

func (l *CertificateLister) List(ctx context.Context, o interface{}) ([]resource.Resource, error) {
	opts := o.(*nuke.ListerOpts)
	client, err := certificatesManagementClient(opts)
	if err != nil {
		return nil, err
	}

	var resources []resource.Resource
	page := ""
	for {
		resp, err := client.ListCertificates(ctx, certificatesmanagement.ListCertificatesRequest{
			CompartmentId: &opts.CompartmentID,
			Page:          strPtrOrNil(page),
		})
		if err != nil {
			return nil, err
		}
		for _, c := range resp.Items {
			switch c.LifecycleState {
			case certificatesmanagement.CertificateLifecycleStateDeleted,
				certificatesmanagement.CertificateLifecycleStateDeleting,
				certificatesmanagement.CertificateLifecycleStateSchedulingDeletion,
				certificatesmanagement.CertificateLifecycleStatePendingDeletion:
				continue
			}
			resources = append(resources, &Certificate{client: client, ID: c.Id, Name: c.Name})
		}
		if resp.OpcNextPage == nil {
			break
		}
		page = *resp.OpcNextPage
	}
	return resources, nil
}

type Certificate struct {
	client certificatesmanagement.CertificatesManagementClient
	ID     *string
	Name   *string
}

// certificateMinRetention is OCI's mandatory minimum delay before a scheduled
// certificate deletion may take effect. The API is explicit about it: scheduling any
// sooner fails with "ScheduledTimeOfDeletion ... is less than minimum 1440 even after
// allowable clock skew 5", i.e. the requested time has to be at least 1440 minutes out
// as the service reads the clock. The five extra minutes are exactly that stated clock
// skew allowance, so this is the earliest time OCI accepts - it is what the service
// itself tolerates between our clock and its own, plus the request's own flight time.
const certificateMinRetention = 24*time.Hour + 5*time.Minute

// A certificate can only ever be *scheduled* for deletion, never deleted outright, so it
// necessarily outlives the nuke - it sits in PENDING_DELETION until the scheduled time
// and OCI then removes it. Letting the service pick leaves it pending for ten days (a
// hand-scheduled deletion in eu-frankfurt-1 came back with exactly that), so an explicit
// earliest-allowed time is used instead to keep the leftover window to ~1 day.
// Consequence: a nuked compartment legitimately still lists one PENDING_DELETION
// certificate per TLS ingress that existed. Nothing blocks a fresh provision, and once
// scheduled the deletion cannot be re-scheduled (IncorrectState) without cancelling
// first, so re-running the nuke leaves the existing schedule alone.
func (r *Certificate) Remove(ctx context.Context) error {
	deleteAt := time.Now().Add(certificateMinRetention)
	_, err := r.client.ScheduleCertificateDeletion(ctx, certificatesmanagement.ScheduleCertificateDeletionRequest{
		CertificateId: r.ID,
		ScheduleCertificateDeletionDetails: certificatesmanagement.ScheduleCertificateDeletionDetails{
			TimeOfDeletion: &common.SDKTime{Time: deleteAt},
		},
	})
	return err
}

func (r *Certificate) Properties() types.Properties {
	return types.NewPropertiesFromStruct(r)
}

func (r *Certificate) String() string {
	if r.Name != nil {
		return *r.Name
	}
	return *r.ID
}
