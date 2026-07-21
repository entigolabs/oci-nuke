package resources

import (
	"context"
	"fmt"
	"sync"

	"github.com/ekristen/libnuke/pkg/registry"
	"github.com/ekristen/libnuke/pkg/resource"
	"github.com/ekristen/libnuke/pkg/types"
	"github.com/oracle/oci-go-sdk/v65/objectstorage"

	"github.com/entigolabs/oci-nuke/pkg/nuke"
)

const BucketResource = "OCIBucket"

func init() {
	registry.Register(&registry.Registration{
		Name:     BucketResource,
		Scope:    nuke.Compartment,
		Resource: &Bucket{},
		Lister:   &BucketLister{},
	})
}

type BucketLister struct{}

func (l *BucketLister) List(ctx context.Context, o interface{}) ([]resource.Resource, error) {
	opts := o.(*nuke.ListerOpts)
	client, err := objectStorageClient(opts)
	if err != nil {
		return nil, err
	}

	ns, err := client.GetNamespace(ctx, objectstorage.GetNamespaceRequest{})
	if err != nil {
		return nil, fmt.Errorf("failed to get object storage namespace: %w", err)
	}

	var resources []resource.Resource
	page := ""
	for {
		resp, err := client.ListBuckets(ctx, objectstorage.ListBucketsRequest{
			NamespaceName: ns.Value,
			CompartmentId: &opts.CompartmentID,
			Page:          strPtrOrNil(page),
		})
		if err != nil {
			return nil, err
		}
		for _, b := range resp.Items {
			resources = append(resources, &Bucket{client: client, Namespace: ns.Value, Name: b.Name})
		}
		if resp.OpcNextPage == nil {
			break
		}
		page = *resp.OpcNextPage
	}
	return resources, nil
}

type Bucket struct {
	client    objectstorage.ObjectStorageClient
	Namespace *string
	Name      *string
}

// Remove empties every object version (including delete markers) before deleting the
// bucket - buckets are created with versioning enabled (see oracle/storage.go in
// entigo-infralib-agent), so a plain bulk-delete leaves old versions behind and the
// bucket delete fails with BucketNotEmpty. Deletes run concurrently: this bucket can
// hold a full mirror of a source repo (thousands of file versions), and deleting them
// one at a time sequentially is what made the original bash nuke script painfully slow.
func (r *Bucket) Remove(ctx context.Context) error {
	const workers = 20

	type item struct {
		name    *string
		version *string
	}
	items := make(chan item, workers)
	errs := make(chan error, workers)
	var wg sync.WaitGroup

	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for it := range items {
				_, err := r.client.DeleteObject(ctx, objectstorage.DeleteObjectRequest{
					NamespaceName: r.Namespace,
					BucketName:    r.Name,
					ObjectName:    it.name,
					VersionId:     it.version,
				})
				if err != nil {
					errs <- err
				}
			}
		}()
	}

	var listErr error
	page := ""
	for {
		resp, err := r.client.ListObjectVersions(ctx, objectstorage.ListObjectVersionsRequest{
			NamespaceName: r.Namespace,
			BucketName:    r.Name,
			Page:          strPtrOrNil(page),
		})
		if err != nil {
			listErr = err
			break
		}
		for _, v := range resp.Items {
			items <- item{name: v.Name, version: v.VersionId}
		}
		if resp.OpcNextPage == nil {
			break
		}
		page = *resp.OpcNextPage
	}
	close(items)
	wg.Wait()
	close(errs)

	if listErr != nil {
		return listErr
	}
	for err := range errs {
		return err
	}

	_, err := r.client.DeleteBucket(ctx, objectstorage.DeleteBucketRequest{
		NamespaceName: r.Namespace,
		BucketName:    r.Name,
	})
	return err
}

func (r *Bucket) Properties() types.Properties {
	return types.NewPropertiesFromStruct(r)
}

func (r *Bucket) String() string {
	return *r.Name
}
