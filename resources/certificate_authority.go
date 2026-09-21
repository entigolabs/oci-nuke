package resources

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/ekristen/libnuke/pkg/registry"
	"github.com/ekristen/libnuke/pkg/resource"
	"github.com/ekristen/libnuke/pkg/types"
	"github.com/oracle/oci-go-sdk/v65/certificatesmanagement"
	"github.com/oracle/oci-go-sdk/v65/common"

	"github.com/entigolabs/oci-nuke/pkg/nuke"
)

const CertificateAuthorityResource = "OCICertificateAuthority"

// The root CA that modules/oracle/dns creates to issue the zone's wildcard certificate,
// signed with the HSM key from modules/oracle/kms.
//
// Deliberately excluded in config.yaml for the Entigo deployment - see the comment there.
// It is implemented anyway so the type exists for anyone who does want it swept, and so
// that a compartment being handed back is not left holding a CA nobody can account for.
func init() {
	registry.Register(&registry.Registration{
		Name:  CertificateAuthorityResource,
		Scope: nuke.Compartment,
		// Certificates the CA issued have to go first, and "first" here can only mean
		// scheduled - a certificate sits in PENDING_DELETION for ~24h. OCI does insist on
		// them being gone rather than merely scheduled, confirmed against the live API:
		//
		//   409 Conflict: The certificate authority (CA) '<name>' ... cannot be scheduled
		//   for deletion because subordinate CAs or certificates exist.
		//
		// So a compartment holding certificates cannot have its CA removed in the same
		// run, whatever order this dependency puts them in. The CA fails, the run exits
		// non-zero, and a second nuke the next day - once the certificates are actually
		// deleted - takes it. That failure is deliberate: a CA left standing is a billed
		// resource, and a run that exits 0 with one still there reads as a clean sweep.
		// Nothing is left in a broken state either way.
		DependsOn: []string{CertificateResource},
		Resource:  &CertificateAuthority{},
		Lister:    &CertificateAuthorityLister{},
	})
}

type CertificateAuthorityLister struct{}

func (l *CertificateAuthorityLister) List(ctx context.Context, o interface{}) ([]resource.Resource, error) {
	opts := o.(*nuke.ListerOpts)
	client, err := certificatesManagementClient(opts)
	if err != nil {
		return nil, err
	}

	var resources []resource.Resource
	page := ""
	for {
		resp, err := client.ListCertificateAuthorities(ctx, certificatesmanagement.ListCertificateAuthoritiesRequest{
			CompartmentId: &opts.CompartmentID,
			Page:          strPtrOrNil(page),
		})
		if err != nil {
			return nil, err
		}
		for _, ca := range resp.Items {
			switch ca.LifecycleState {
			case certificatesmanagement.CertificateAuthorityLifecycleStateDeleted,
				certificatesmanagement.CertificateAuthorityLifecycleStateDeleting,
				certificatesmanagement.CertificateAuthorityLifecycleStateSchedulingDeletion:
				continue
			case certificatesmanagement.CertificateAuthorityLifecycleStatePendingDeletion:
				if !scheduledLaterThanNecessary(ca.TimeOfDeletion, certificateAuthorityMinRetention) {
					continue
				}
			}
			resources = append(resources, &CertificateAuthority{client: client, ID: ca.Id, Name: ca.Name})
		}
		if resp.OpcNextPage == nil {
			break
		}
		page = *resp.OpcNextPage
	}
	return resources, nil
}

type CertificateAuthority struct {
	client certificatesmanagement.CertificatesManagementClient
	ID     *string
	Name   *string
}

// A CA is NOT the 24 hours a certificate gets - it is 7 days, the same as a vault or key.
// The SDK documents no minimum at all, and assuming the certificate's 1440 minutes earns
// "ScheduledTimeOfDeletion ... is less than minimum 10080 even after allowable clock skew
// 5" from the live API, so the earliest it accepts is 10080 minutes plus that skew.
const certificateAuthorityMinRetention = 7*24*time.Hour + scheduledDeletionSkew

// Same handling as a certificate, for the same reasons - see scheduled_deletion.go.
func (r *CertificateAuthority) Remove(ctx context.Context) error {
	return deletionSchedule{
		minRetention: certificateAuthorityMinRetention,
		read: func(ctx context.Context) (string, *common.SDKTime, error) {
			resp, err := r.client.GetCertificateAuthority(ctx, certificatesmanagement.GetCertificateAuthorityRequest{
				CertificateAuthorityId: r.ID,
			})
			if err != nil {
				return "", nil, err
			}
			return string(resp.LifecycleState), resp.TimeOfDeletion, nil
		},
		schedule: func(ctx context.Context, at time.Time) error {
			_, err := r.client.ScheduleCertificateAuthorityDeletion(ctx, certificatesmanagement.ScheduleCertificateAuthorityDeletionRequest{
				CertificateAuthorityId: r.ID,
				ScheduleCertificateAuthorityDeletionDetails: certificatesmanagement.ScheduleCertificateAuthorityDeletionDetails{
					TimeOfDeletion: &common.SDKTime{Time: at},
				},
			})
			return err
		},
		cancel: func(ctx context.Context) error {
			_, err := r.client.CancelCertificateAuthorityDeletion(ctx, certificatesmanagement.CancelCertificateAuthorityDeletionRequest{
				CertificateAuthorityId: r.ID,
			})
			return err
		},
		blockedBy: certificateAuthorityBlockedByCertificates,
	}.ensure(ctx)
}

func (r *CertificateAuthority) Properties() types.Properties {
	return types.NewPropertiesFromStruct(r)
}

func (r *CertificateAuthority) String() string {
	if r.Name != nil {
		return *r.Name
	}
	return *r.ID
}

// A CA cannot be scheduled for deletion while any certificate it issued still exists, and a
// certificate that is itself PENDING_DELETION counts as existing for the whole of its own
// retention. So a CA and its certificates can never go in one run: the certificates are
// scheduled, and the CA follows once their dates pass.
//
// That is the expected order of events rather than something to fix, so it is reported as a
// hold. The service states it plainly enough to match on:
//
//	cannot be scheduled for deletion because subordinate CAs or certificates exist
func certificateAuthorityBlockedByCertificates(err error) (string, bool) {
	var svcErr common.ServiceError
	if !errors.As(err, &svcErr) || svcErr.GetHTTPStatusCode() != http.StatusConflict {
		return "", false
	}
	if !strings.Contains(svcErr.GetMessage(), "subordinate CAs or certificates exist") {
		return "", false
	}
	return "certificates it issued are still within their own deletion retention; " +
		"the CA can be scheduled once they are gone", true
}
