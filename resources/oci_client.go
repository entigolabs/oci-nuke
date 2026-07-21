// Package resources implements libnuke Resource/Lister pairs for the OCI resource
// types the Entigo Infralib Oracle bootstrap + terraform modules create. Each file
// mirrors ekristen/gcp-nuke's per-resource-file convention.
package resources

import (
	"github.com/entigolabs/oci-nuke/pkg/nuke"

	"github.com/oracle/oci-go-sdk/v65/containerinstances"
	"github.com/oracle/oci-go-sdk/v65/core"
	"github.com/oracle/oci-go-sdk/v65/devops"
	"github.com/oracle/oci-go-sdk/v65/identity"
	"github.com/oracle/oci-go-sdk/v65/logging"
	"github.com/oracle/oci-go-sdk/v65/objectstorage"
	"github.com/oracle/oci-go-sdk/v65/ons"
)

func virtualNetworkClient(o *nuke.ListerOpts) (core.VirtualNetworkClient, error) {
	client, err := core.NewVirtualNetworkClientWithConfigurationProvider(o.Provider)
	if err != nil {
		return client, err
	}
	client.SetRegion(o.Region)
	return client, nil
}

func objectStorageClient(o *nuke.ListerOpts) (objectstorage.ObjectStorageClient, error) {
	client, err := objectstorage.NewObjectStorageClientWithConfigurationProvider(o.Provider)
	if err != nil {
		return client, err
	}
	client.SetRegion(o.Region)
	return client, nil
}

func loggingManagementClient(o *nuke.ListerOpts) (logging.LoggingManagementClient, error) {
	client, err := logging.NewLoggingManagementClientWithConfigurationProvider(o.Provider)
	if err != nil {
		return client, err
	}
	client.SetRegion(o.Region)
	return client, nil
}

func identityClient(o *nuke.ListerOpts) (identity.IdentityClient, error) {
	client, err := identity.NewIdentityClientWithConfigurationProvider(o.Provider)
	if err != nil {
		return client, err
	}
	client.SetRegion(o.Region)
	return client, nil
}

func containerInstanceClient(o *nuke.ListerOpts) (containerinstances.ContainerInstanceClient, error) {
	client, err := containerinstances.NewContainerInstanceClientWithConfigurationProvider(o.Provider)
	if err != nil {
		return client, err
	}
	client.SetRegion(o.Region)
	return client, nil
}

func devopsClient(o *nuke.ListerOpts) (devops.DevopsClient, error) {
	client, err := devops.NewDevopsClientWithConfigurationProvider(o.Provider)
	if err != nil {
		return client, err
	}
	client.SetRegion(o.Region)
	return client, nil
}

func onsClient(o *nuke.ListerOpts) (ons.NotificationControlPlaneClient, error) {
	client, err := ons.NewNotificationControlPlaneClientWithConfigurationProvider(o.Provider)
	if err != nil {
		return client, err
	}
	client.SetRegion(o.Region)
	return client, nil
}
