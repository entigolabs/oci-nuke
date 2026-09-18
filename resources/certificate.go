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
				certificatesmanagement.CertificateLifecycleStateSchedulingDeletion:
				continue
			case certificatesmanagement.CertificateLifecycleStatePendingDeletion:
				if !scheduledLaterThanNecessary(c.TimeOfDeletion) {
					continue
				}
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

// certificateRescheduleSlack is how much later than certificateMinRetention an existing
// schedule has to be before it is worth cancelling and re-scheduling. It only has to cover
// the drift between a certificate scheduled at the start of a run and a listing later in
// the same run, both measured against certificateMinRetention from "now".
const certificateRescheduleSlack = time.Hour

// A certificate can only ever be *scheduled* for deletion, never deleted outright, so it
// necessarily outlives the nuke - it sits in PENDING_DELETION until the scheduled time
// and OCI then removes it. Letting the service pick the time leaves it pending for ten
// days (that is what a certificate deleted without one came back with in eu-frankfurt-1,
// not the thirty days this comment used to claim), so an explicit earliest-allowed time
// is used instead to keep the leftover window to ~1 day.
// Consequence: a nuked compartment legitimately still lists one PENDING_DELETION
// certificate per TLS ingress that existed. Nothing blocks a fresh provision.
// A schedule that is already in place is honoured only if it is not later than what this
// tool would ask for. A certificate deleted without an explicit time - by the console, by
// terraform, or by a build of this tool from before the time was set - sits in
// PENDING_DELETION for ten days, and re-running the nuke used to leave that alone. It is
// now cancelled and re-scheduled at the earliest allowed time instead.
func (r *Certificate) Remove(ctx context.Context) error {
	err := r.scheduleDeletion(ctx)
	if err == nil || !isIncorrectState(err) {
		return err
	}
	// Already pending deletion on a date we want moved: the schedule cannot be rewritten
	// in place, it has to be cancelled first. Cancelling is asynchronous, so the schedule
	// that follows may still hit CANCELLING_DELETION - libnuke retries Remove, and the
	// retry finds an ACTIVE certificate and schedules it normally.
	if _, cancelErr := r.client.CancelCertificateDeletion(ctx, certificatesmanagement.CancelCertificateDeletionRequest{
		CertificateId: r.ID,
	}); cancelErr != nil {
		return cancelErr
	}
	return r.scheduleDeletion(ctx)
}

func (r *Certificate) scheduleDeletion(ctx context.Context) error {
	deleteAt := time.Now().Add(certificateMinRetention)
	_, err := r.client.ScheduleCertificateDeletion(ctx, certificatesmanagement.ScheduleCertificateDeletionRequest{
		CertificateId: r.ID,
		ScheduleCertificateDeletionDetails: certificatesmanagement.ScheduleCertificateDeletionDetails{
			TimeOfDeletion: &common.SDKTime{Time: deleteAt},
		},
	})
	return err
}

// scheduledLaterThanNecessary reports whether an existing schedule is far enough out to be
// worth pulling in. The slack matters for more than politeness: libnuke only marks an item
// finished once the lister stops returning it, so a certificate this keeps saying yes to is
// one the run waits on forever (the same trap the default route table fell into). Anything
// this tool scheduled sits at certificateMinRetention from the moment it ran and so drops
// out on the next pass; a ten-day schedule left by the console, terraform or an older build
// of this tool does not.
func scheduledLaterThanNecessary(timeOfDeletion *common.SDKTime) bool {
	if timeOfDeletion == nil {
		return false
	}
	return timeOfDeletion.After(time.Now().Add(certificateMinRetention + certificateRescheduleSlack))
}

func isIncorrectState(err error) bool {
	svcErr, ok := common.IsServiceError(err)
	return ok && (svcErr.GetCode() == "IncorrectState" || svcErr.GetHTTPStatusCode() == 409)
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
