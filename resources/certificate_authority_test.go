package resources

import (
	"testing"
	"time"

	"github.com/ekristen/libnuke/pkg/resource"
	"github.com/oracle/oci-go-sdk/v65/certificatesmanagement"
	"github.com/oracle/oci-go-sdk/v65/common"
)

// libnuke finds these hooks by type assertion, so a signature that drifts from its interfaces
// would silently turn them off; these fail the build instead.
var (
	_ resource.Deferrer          = &CertificateAuthority{}
	_ resource.Blocker           = &CertificateAuthority{}
	_ resource.Blocker           = &Key{}
	_ resource.Blocker           = &Certificate{}
	_ resource.DeletionScheduler = &CertificateAuthority{}
	_ resource.DeletionScheduler = &Certificate{}
	_ resource.DeletionScheduler = &Key{}
	_ resource.DeletionScheduler = &Vault{}
)

func TestCertificateBlocksIssuer(t *testing.T) {
	now := time.Now()
	at := func(d time.Duration) *common.SDKTime { return &common.SDKTime{Time: now.Add(d)} }

	for _, tc := range []struct {
		name    string
		state   certificatesmanagement.CertificateLifecycleStateEnum
		deleted *common.SDKTime
		want    bool
	}{
		{"active", certificatesmanagement.CertificateLifecycleStateActive, nil, true},
		{"failed", certificatesmanagement.CertificateLifecycleStateFailed, nil, true},
		{"pending deletion", certificatesmanagement.CertificateLifecycleStatePendingDeletion, at(20 * time.Hour), true},
		{"deleted an hour ago", certificatesmanagement.CertificateLifecycleStateDeleted, at(-time.Hour), true},
		{"deleted with no date", certificatesmanagement.CertificateLifecycleStateDeleted, nil, true},
		{"deleted over a day ago", certificatesmanagement.CertificateLifecycleStateDeleted, at(-25 * time.Hour), false},
	} {
		if got := certificateBlocksIssuer(tc.state, tc.deleted, now); got != tc.want {
			t.Errorf("%s: got %t, want %t", tc.name, got, tc.want)
		}
	}
}
