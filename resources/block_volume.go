package resources

import (
	"context"

	"github.com/ekristen/libnuke/pkg/registry"
	"github.com/ekristen/libnuke/pkg/resource"
	"github.com/ekristen/libnuke/pkg/types"
	"github.com/oracle/oci-go-sdk/v65/core"

	"github.com/entigolabs/oci-nuke/pkg/nuke"
)

const BlockVolumeResource = "OCIBlockVolume"

func init() {
	registry.Register(&registry.Registration{
		Name:      BlockVolumeResource,
		Scope:     nuke.Compartment,
		Resource:  &BlockVolume{},
		Lister:    &BlockVolumeLister{},
		DependsOn: []string{OkeNodePoolResource},
	})
}

type BlockVolumeLister struct{}

func (l *BlockVolumeLister) List(ctx context.Context, o interface{}) ([]resource.Resource, error) {
	opts := o.(*nuke.ListerOpts)
	client, err := blockstorageClient(opts)
	if err != nil {
		return nil, err
	}

	var resources []resource.Resource
	page := ""
	for {
		resp, err := client.ListVolumes(ctx, core.ListVolumesRequest{
			CompartmentId: &opts.CompartmentID,
			Page:          strPtrOrNil(page),
		})
		if err != nil {
			return nil, err
		}
		for _, v := range resp.Items {
			switch v.LifecycleState {
			case core.VolumeLifecycleStateTerminated, core.VolumeLifecycleStateTerminating:
				continue
			}
			resources = append(resources, &BlockVolume{client: client, ID: v.Id, Name: v.DisplayName})
		}
		if resp.OpcNextPage == nil {
			break
		}
		page = *resp.OpcNextPage
	}
	return resources, nil
}

type BlockVolume struct {
	client core.BlockstorageClient
	ID     *string
	Name   *string
}

func (r *BlockVolume) Remove(ctx context.Context) error {
	_, err := r.client.DeleteVolume(ctx, core.DeleteVolumeRequest{VolumeId: r.ID})
	return err
}

func (r *BlockVolume) Properties() types.Properties {
	return types.NewPropertiesFromStruct(r)
}

func (r *BlockVolume) String() string {
	if r.Name != nil {
		return *r.Name
	}
	return *r.ID
}
