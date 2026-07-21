package resources

import (
	"context"

	"github.com/ekristen/libnuke/pkg/registry"
	"github.com/ekristen/libnuke/pkg/resource"
	"github.com/ekristen/libnuke/pkg/types"
	"github.com/oracle/oci-go-sdk/v65/ons"

	"github.com/entigolabs/oci-nuke/pkg/nuke"
)

const OnsTopicResource = "OCIOnsTopic"

func init() {
	registry.Register(&registry.Registration{
		Name:     OnsTopicResource,
		Scope:    nuke.Compartment,
		Resource: &OnsTopic{},
		Lister:   &OnsTopicLister{},
	})
}

type OnsTopicLister struct{}

func (l *OnsTopicLister) List(ctx context.Context, o interface{}) ([]resource.Resource, error) {
	opts := o.(*nuke.ListerOpts)
	client, err := onsClient(opts)
	if err != nil {
		return nil, err
	}

	var resources []resource.Resource
	page := ""
	for {
		resp, err := client.ListTopics(ctx, ons.ListTopicsRequest{
			CompartmentId: &opts.CompartmentID,
			Page:          strPtrOrNil(page),
		})
		if err != nil {
			return nil, err
		}
		for _, t := range resp.Items {
			if t.LifecycleState == ons.NotificationTopicSummaryLifecycleStateDeleting {
				continue
			}
			resources = append(resources, &OnsTopic{client: client, ID: t.TopicId, Name: t.Name})
		}
		if resp.OpcNextPage == nil {
			break
		}
		page = *resp.OpcNextPage
	}
	return resources, nil
}

type OnsTopic struct {
	client ons.NotificationControlPlaneClient
	ID     *string
	Name   *string
}

func (r *OnsTopic) Remove(ctx context.Context) error {
	_, err := r.client.DeleteTopic(ctx, ons.DeleteTopicRequest{TopicId: r.ID})
	return err
}

func (r *OnsTopic) Properties() types.Properties {
	return types.NewPropertiesFromStruct(r)
}

func (r *OnsTopic) String() string {
	if r.Name != nil {
		return *r.Name
	}
	return *r.ID
}
