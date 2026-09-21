package resources

import (
	"context"

	"github.com/ekristen/libnuke/pkg/registry"
	"github.com/ekristen/libnuke/pkg/resource"
	"github.com/ekristen/libnuke/pkg/types"
	"github.com/oracle/oci-go-sdk/v65/core"

	"github.com/entigolabs/oci-nuke/pkg/nuke"
)

const OrphanedVnicResource = "OCIOrphanedVnic"

// A node killed while it still has running pods (e.g. by OkeNodePoolResource, or a
// force-terminated instance) can leave a VCN-native pod's secondary VNIC behind with no
// VnicAttachment at all - the CNI never got the chance to release it. There is no
// ListVnics or DeleteVnic API; a VNIC is only ever discovered via its attachment or its
// private IP, and only ever released via detaching its private IP. This lister finds
// exactly the ones with no attachment (never a VNIC still in use) and releases them via
// PrivateIpVnicDetach, the same call OCI's own pod-networking CNI uses for this.
func init() {
	registry.Register(&registry.Registration{
		Name:     OrphanedVnicResource,
		Scope:    nuke.Compartment,
		Resource: &OrphanedVnic{},
		Lister:   &OrphanedVnicLister{},
	})
}

type OrphanedVnicLister struct{}

func (l *OrphanedVnicLister) List(ctx context.Context, o interface{}) ([]resource.Resource, error) {
	opts := o.(*nuke.ListerOpts)
	computeClient, err := computeClient(opts)
	if err != nil {
		return nil, err
	}
	networkClient, err := virtualNetworkClient(opts)
	if err != nil {
		return nil, err
	}

	attached, err := attachedVnicIDs(ctx, computeClient, opts.CompartmentID)
	if err != nil {
		return nil, err
	}

	var resources []resource.Resource
	subnetPage := ""
	for {
		subnetResp, err := networkClient.ListSubnets(ctx, core.ListSubnetsRequest{
			CompartmentId: &opts.CompartmentID,
			Page:          strPtrOrNil(subnetPage),
		})
		if err != nil {
			return nil, err
		}
		for _, subnet := range subnetResp.Items {
			ipPage := ""
			for {
				ipResp, err := networkClient.ListPrivateIps(ctx, core.ListPrivateIpsRequest{
					SubnetId: subnet.Id,
					Page:     strPtrOrNil(ipPage),
				})
				if err != nil {
					return nil, err
				}
				for _, ip := range ipResp.Items {
					if ip.VnicId == nil || ip.IsPrimary == nil || !*ip.IsPrimary {
						continue
					}
					if attached[*ip.VnicId] {
						continue
					}
					resources = append(resources, &OrphanedVnic{client: networkClient, PrivateIpID: ip.Id, VnicID: ip.VnicId})
				}
				if ipResp.OpcNextPage == nil {
					break
				}
				ipPage = *ipResp.OpcNextPage
			}
		}
		if subnetResp.OpcNextPage == nil {
			break
		}
		subnetPage = *subnetResp.OpcNextPage
	}
	return resources, nil
}

// attachedVnicIDs is every VNIC currently reachable through a live attachment - anything
// not in this set has no instance or pod left to belong to.
func attachedVnicIDs(ctx context.Context, client core.ComputeClient, compartmentID string) (map[string]bool, error) {
	attached := make(map[string]bool)
	page := ""
	for {
		resp, err := client.ListVnicAttachments(ctx, core.ListVnicAttachmentsRequest{
			CompartmentId: &compartmentID,
			Page:          strPtrOrNil(page),
		})
		if err != nil {
			return nil, err
		}
		for _, a := range resp.Items {
			switch a.LifecycleState {
			case core.VnicAttachmentLifecycleStateDetached, core.VnicAttachmentLifecycleStateDetaching:
				continue
			}
			if a.VnicId != nil {
				attached[*a.VnicId] = true
			}
		}
		if resp.OpcNextPage == nil {
			break
		}
		page = *resp.OpcNextPage
	}
	return attached, nil
}

type OrphanedVnic struct {
	client      core.VirtualNetworkClient
	PrivateIpID *string
	VnicID      *string
}

func (r *OrphanedVnic) Remove(ctx context.Context) error {
	_, err := r.client.PrivateIpVnicDetach(ctx, core.PrivateIpVnicDetachRequest{PrivateIpId: r.PrivateIpID})
	return err
}

func (r *OrphanedVnic) Properties() types.Properties {
	return types.NewPropertiesFromStruct(r)
}

func (r *OrphanedVnic) String() string {
	if r.VnicID != nil {
		return *r.VnicID
	}
	return *r.PrivateIpID
}
