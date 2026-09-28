package resources

import (
	"context"
	"errors"
	"fmt"
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
		// run, whatever order this dependency puts them in. The scan defers the CA - see
		// Defer - and a nuke the next day, once the certificates are actually deleted,
		// takes it. The run still succeeds, and names the deferred CA at the end, so a CA
		// left standing is reported rather than hidden.
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
			resources = append(resources, &CertificateAuthority{
				client: client, compartmentID: opts.CompartmentID, kmsKeyID: ca.KmsKeyId,
				ID: ca.Id, Name: ca.Name,
			})
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
	// Unexported so that they stay out of Properties; the issuer filters need the
	// compartment, and Blocks the signing key.
	compartmentID string
	kmsKeyID      *string
	ID            *string
	Name          *string
}

// Blocks names the CA's signing key: a deferred CA holds back its own key, not every key in
// the compartment.
func (r *CertificateAuthority) Blocks() []resource.Ref {
	if r.kmsKeyID == nil {
		return nil
	}
	return []resource.Ref{{Type: KeyResource, ID: *r.kmsKeyID}}
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

// A certificate stops blocking its issuer only some time after it is DELETED, not when it
// is scheduled: a certificate in PENDING_DELETION, or deleted less than this long ago,
// still earns the 409 below.
const certificateBlocksIssuerAfterDeletion = 24 * time.Hour

// Defer checks, before anything is removed, what certificateAuthorityBlockedByCertificates
// would otherwise learn from a refused schedule: whether certificates or subordinate CAs
// the CA issued still exist. Any that are still ACTIVE are scheduled in this same run, by
// DependsOn, and then block the CA just as long. So the CA is deferred either way, and the
// reason names what blocks it and in what state. A certificate a filter keeps is never
// scheduled at all; the scan sees that through Certificate.Blocks and keeps the CA instead.
//
// Only the CA's own compartment is searched: the service rejects an issuer filter without
// one. A blocker elsewhere is still caught when the schedule is refused.
func (r *CertificateAuthority) Defer(ctx context.Context) (string, bool, error) {
	var blockers []string

	page := ""
	for {
		resp, err := r.client.ListCertificates(ctx, certificatesmanagement.ListCertificatesRequest{
			CompartmentId:                &r.compartmentID,
			IssuerCertificateAuthorityId: r.ID,
			Page:                         strPtrOrNil(page),
		})
		if err != nil {
			return "", false, err
		}
		for _, c := range resp.Items {
			if certificateBlocksIssuer(c.LifecycleState, c.TimeOfDeletion, time.Now()) {
				blockers = append(blockers, fmt.Sprintf("certificate %s (%s)", strOr(c.Name, "?"), c.LifecycleState))
			}
		}
		if resp.OpcNextPage == nil {
			break
		}
		page = *resp.OpcNextPage
	}

	page = ""
	for {
		resp, err := r.client.ListCertificateAuthorities(ctx, certificatesmanagement.ListCertificateAuthoritiesRequest{
			CompartmentId:                &r.compartmentID,
			IssuerCertificateAuthorityId: r.ID,
			Page:                         strPtrOrNil(page),
		})
		if err != nil {
			return "", false, err
		}
		for _, ca := range resp.Items {
			if ca.LifecycleState != certificatesmanagement.CertificateAuthorityLifecycleStateDeleted {
				blockers = append(blockers, fmt.Sprintf("subordinate CA %s (%s)", strOr(ca.Name, "?"), ca.LifecycleState))
			}
		}
		if resp.OpcNextPage == nil {
			break
		}
		page = *resp.OpcNextPage
	}

	if len(blockers) == 0 {
		return "", false, nil
	}
	return "blocked until deleted: " + strings.Join(blockers, ", "), true, nil
}

// certificateBlocksIssuer reports whether a certificate in this state still prevents its
// issuing CA from being scheduled for deletion.
func certificateBlocksIssuer(state certificatesmanagement.CertificateLifecycleStateEnum, deleted *common.SDKTime, now time.Time) bool {
	if state != certificatesmanagement.CertificateLifecycleStateDeleted {
		return true
	}
	return deleted == nil || now.Before(deleted.Add(certificateBlocksIssuerAfterDeletion))
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
// That is the expected order of events rather than something to fix, so the CA is deferred
// to a later run. Defer normally finds this at scan time; this is the fallback for a
// blocker it could not see. The service states it plainly enough to match on - and the 409's code is
// only the generic "Conflict", so the message is the one thing that tells this refusal apart:
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
