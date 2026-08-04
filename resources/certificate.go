package resources

import (
	"context"

	"github.com/ekristen/libnuke/pkg/registry"
	"github.com/ekristen/libnuke/pkg/resource"
	"github.com/ekristen/libnuke/pkg/types"
	"github.com/oracle/oci-go-sdk/v65/certificatesmanagement"

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

func (r *Certificate) Remove(ctx context.Context) error {
	// No TimeOfDeletion = delete as soon as the service processes it (certificates have
	// no mandatory retention window, unlike KMS vaults).
	_, err := r.client.ScheduleCertificateDeletion(ctx, certificatesmanagement.ScheduleCertificateDeletionRequest{
		CertificateId:                      r.ID,
		ScheduleCertificateDeletionDetails: certificatesmanagement.ScheduleCertificateDeletionDetails{},
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
