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
				if !scheduledLaterThanNecessary(c.TimeOfDeletion, certificateMinRetention) {
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
// allowable clock skew 5", so the earliest it accepts is 1440 minutes plus the five
// minutes of skew it says it allows for.
const certificateMinRetention = 24*time.Hour + scheduledDeletionSkew

// A certificate can only ever be *scheduled* for deletion, never deleted outright, so it
// necessarily outlives the nuke - it sits in PENDING_DELETION until the scheduled date
// and OCI removes it then. A nuked compartment therefore legitimately still lists one
// PENDING_DELETION certificate per TLS ingress that existed, and nothing blocks a fresh
// provision in the meantime. What the run does guarantee is the date: see
// scheduled_deletion.go for why it is read back rather than assumed, and why a
// certificate already scheduled further out than that gets its schedule replaced.
func (r *Certificate) Remove(ctx context.Context) error {
	return deletionSchedule{
		minRetention: certificateMinRetention,
		read: func(ctx context.Context) (string, *common.SDKTime, error) {
			resp, err := r.client.GetCertificate(ctx, certificatesmanagement.GetCertificateRequest{
				CertificateId: r.ID,
			})
			if err != nil {
				return "", nil, err
			}
			return string(resp.LifecycleState), resp.TimeOfDeletion, nil
		},
		schedule: func(ctx context.Context, at time.Time) error {
			_, err := r.client.ScheduleCertificateDeletion(ctx, certificatesmanagement.ScheduleCertificateDeletionRequest{
				CertificateId: r.ID,
				ScheduleCertificateDeletionDetails: certificatesmanagement.ScheduleCertificateDeletionDetails{
					TimeOfDeletion: &common.SDKTime{Time: at},
				},
			})
			return err
		},
		cancel: func(ctx context.Context) error {
			_, err := r.client.CancelCertificateDeletion(ctx, certificatesmanagement.CancelCertificateDeletionRequest{
				CertificateId: r.ID,
			})
			return err
		},
	}.ensure(ctx)
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
