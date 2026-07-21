package resources

import (
	"context"

	"github.com/ekristen/libnuke/pkg/registry"
	"github.com/ekristen/libnuke/pkg/resource"
	"github.com/ekristen/libnuke/pkg/types"
	"github.com/oracle/oci-go-sdk/v65/logging"

	"github.com/entigolabs/oci-nuke/pkg/nuke"
)

const LogGroupResource = "OCILogGroup"

func init() {
	registry.Register(&registry.Registration{
		Name:      LogGroupResource,
		Scope:     nuke.Compartment,
		Resource:  &LogGroup{},
		Lister:    &LogGroupLister{},
		DependsOn: []string{LogResource},
	})
}

type LogGroupLister struct{}

func (l *LogGroupLister) List(ctx context.Context, o interface{}) ([]resource.Resource, error) {
	opts := o.(*nuke.ListerOpts)
	client, err := loggingManagementClient(opts)
	if err != nil {
		return nil, err
	}

	var resources []resource.Resource
	page := ""
	for {
		resp, err := client.ListLogGroups(ctx, logging.ListLogGroupsRequest{
			CompartmentId: &opts.CompartmentID,
			Page:          strPtrOrNil(page),
		})
		if err != nil {
			return nil, err
		}
		for _, group := range resp.Items {
			if group.LifecycleState == logging.LogGroupLifecycleStateDeleting {
				continue
			}
			resources = append(resources, &LogGroup{client: client, ID: group.Id, Name: group.DisplayName})
		}
		if resp.OpcNextPage == nil {
			break
		}
		page = *resp.OpcNextPage
	}
	return resources, nil
}

type LogGroup struct {
	client logging.LoggingManagementClient
	ID     *string
	Name   *string
}

func (r *LogGroup) Remove(ctx context.Context) error {
	_, err := r.client.DeleteLogGroup(ctx, logging.DeleteLogGroupRequest{LogGroupId: r.ID})
	return err
}

func (r *LogGroup) Properties() types.Properties {
	return types.NewPropertiesFromStruct(r)
}

func (r *LogGroup) String() string {
	if r.Name != nil {
		return *r.Name
	}
	return *r.ID
}
