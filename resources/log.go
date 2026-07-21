package resources

import (
	"context"

	"github.com/ekristen/libnuke/pkg/registry"
	"github.com/ekristen/libnuke/pkg/resource"
	"github.com/ekristen/libnuke/pkg/types"
	"github.com/oracle/oci-go-sdk/v65/logging"

	"github.com/entigolabs/oci-nuke/pkg/nuke"
)

const LogResource = "OCILog"

func init() {
	registry.Register(&registry.Registration{
		Name:     LogResource,
		Scope:    nuke.Compartment,
		Resource: &Log{},
		Lister:   &LogLister{},
	})
}

type LogLister struct{}

func (l *LogLister) List(ctx context.Context, o interface{}) ([]resource.Resource, error) {
	opts := o.(*nuke.ListerOpts)
	client, err := loggingManagementClient(opts)
	if err != nil {
		return nil, err
	}

	var resources []resource.Resource
	groupPage := ""
	for {
		groupResp, err := client.ListLogGroups(ctx, logging.ListLogGroupsRequest{
			CompartmentId: &opts.CompartmentID,
			Page:          strPtrOrNil(groupPage),
		})
		if err != nil {
			return nil, err
		}
		for _, group := range groupResp.Items {
			if group.LifecycleState == logging.LogGroupLifecycleStateDeleting {
				continue
			}
			logPage := ""
			for {
				logResp, err := client.ListLogs(ctx, logging.ListLogsRequest{
					LogGroupId: group.Id,
					Page:       strPtrOrNil(logPage),
				})
				if err != nil {
					return nil, err
				}
				for _, lg := range logResp.Items {
					if lg.LifecycleState == logging.LogLifecycleStateDeleting {
						continue
					}
					resources = append(resources, &Log{
						client: client, LogGroupID: group.Id, ID: lg.Id, Name: lg.DisplayName,
					})
				}
				if logResp.OpcNextPage == nil {
					break
				}
				logPage = *logResp.OpcNextPage
			}
		}
		if groupResp.OpcNextPage == nil {
			break
		}
		groupPage = *groupResp.OpcNextPage
	}
	return resources, nil
}

type Log struct {
	client     logging.LoggingManagementClient
	LogGroupID *string
	ID         *string
	Name       *string
}

func (r *Log) Remove(ctx context.Context) error {
	_, err := r.client.DeleteLog(ctx, logging.DeleteLogRequest{LogGroupId: r.LogGroupID, LogId: r.ID})
	return err
}

func (r *Log) Properties() types.Properties {
	return types.NewPropertiesFromStruct(r)
}

func (r *Log) String() string {
	if r.Name != nil {
		return *r.Name
	}
	return *r.ID
}
