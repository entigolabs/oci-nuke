// Package resources implements libnuke Resource/Lister pairs for the OCI resource
// types the Entigo Infralib Oracle bootstrap + terraform modules create. Each file
// mirrors ekristen/gcp-nuke's per-resource-file convention.
package resources

import (
	"github.com/entigolabs/oci-nuke/pkg/nuke"

	"github.com/oracle/oci-go-sdk/v65/certificatesmanagement"
	"github.com/oracle/oci-go-sdk/v65/containerengine"
	"github.com/oracle/oci-go-sdk/v65/containerinstances"
	"github.com/oracle/oci-go-sdk/v65/core"
	"github.com/oracle/oci-go-sdk/v65/devops"
	"github.com/oracle/oci-go-sdk/v65/dns"
	"github.com/oracle/oci-go-sdk/v65/identity"
	"github.com/oracle/oci-go-sdk/v65/keymanagement"
	"github.com/oracle/oci-go-sdk/v65/loadbalancer"
	"github.com/oracle/oci-go-sdk/v65/logging"
	"github.com/oracle/oci-go-sdk/v65/networkloadbalancer"
	"github.com/oracle/oci-go-sdk/v65/objectstorage"
	"github.com/oracle/oci-go-sdk/v65/ons"
)

func computeClient(o *nuke.ListerOpts) (core.ComputeClient, error) {
	client, err := core.NewComputeClientWithConfigurationProvider(o.Provider)
	if err != nil {
		return client, err
	}
	client.SetRegion(o.Region)
	return client, nil
}

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

func certificatesManagementClient(o *nuke.ListerOpts) (certificatesmanagement.CertificatesManagementClient, error) {
	client, err := certificatesmanagement.NewCertificatesManagementClientWithConfigurationProvider(o.Provider)
	if err != nil {
		return client, err
	}
	client.SetRegion(o.Region)
	return client, nil
}

func loadBalancerClient(o *nuke.ListerOpts) (loadbalancer.LoadBalancerClient, error) {
	client, err := loadbalancer.NewLoadBalancerClientWithConfigurationProvider(o.Provider)
	if err != nil {
		return client, err
	}
	client.SetRegion(o.Region)
	return client, nil
}

func networkLoadBalancerClient(o *nuke.ListerOpts) (networkloadbalancer.NetworkLoadBalancerClient, error) {
	client, err := networkloadbalancer.NewNetworkLoadBalancerClientWithConfigurationProvider(o.Provider)
	if err != nil {
		return client, err
	}
	client.SetRegion(o.Region)
	return client, nil
}

func containerEngineClient(o *nuke.ListerOpts) (containerengine.ContainerEngineClient, error) {
	client, err := containerengine.NewContainerEngineClientWithConfigurationProvider(o.Provider)
	if err != nil {
		return client, err
	}
	client.SetRegion(o.Region)
	return client, nil
}

func kmsVaultClient(o *nuke.ListerOpts) (keymanagement.KmsVaultClient, error) {
	client, err := keymanagement.NewKmsVaultClientWithConfigurationProvider(o.Provider)
	if err != nil {
		return client, err
	}
	client.SetRegion(o.Region)
	return client, nil
}

// Unlike every other client here this one is per-vault, not per-region: key operations go
// to the vault's own management endpoint, which is only known once the vault is listed.
// SetRegion is therefore deliberately not called - the endpoint already carries it.
func kmsManagementClient(o *nuke.ListerOpts, endpoint string) (keymanagement.KmsManagementClient, error) {
	return keymanagement.NewKmsManagementClientWithConfigurationProvider(o.Provider, endpoint)
}

func dnsClient(o *nuke.ListerOpts) (dns.DnsClient, error) {
	client, err := dns.NewDnsClientWithConfigurationProvider(o.Provider)
	if err != nil {
		return client, err
	}
	client.SetRegion(o.Region)
	return client, nil
}
