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
		// Certificates the CA issued have to go first. Note that "first" here only means
		// scheduled - a certificate sits in PENDING_DELETION for ~24h - so if OCI insists
		// on the certificates being fully gone rather than merely scheduled, the CA will
		// fail this run and needs a second nuke the next day. Nothing is left in a broken
		// state either way.
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
