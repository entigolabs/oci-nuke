package nuke

// entigo patch: tests for ErrDeferResource / ItemStateDeferred and the scan-time
// resource.Deferrer / resource.DeletionScheduler hooks.

import (
	"context"
	"testing"
	"time"

	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"

	liberrors "github.com/ekristen/libnuke/pkg/errors"
	"github.com/ekristen/libnuke/pkg/filter"
	"github.com/ekristen/libnuke/pkg/queue"
	"github.com/ekristen/libnuke/pkg/registry"
	"github.com/ekristen/libnuke/pkg/resource"
	"github.com/ekristen/libnuke/pkg/scanner"
	"github.com/ekristen/libnuke/pkg/types"
)

// MaxWaitRetries is set so that treating a deferred item as still pending fails the run with
// "max wait retries of 3 exceeded" - the production failure this state exists to prevent -
// rather than retrying forever and hanging the test. WaitOnDependencies matches oci-nuke's
// run command, and without it DependsOn is ignored altogether.
var testParametersDefer = &Parameters{
	Force:              true,
	ForceSleep:         3,
	Quiet:              true,
	NoDryRun:           true,
	MaxWaitRetries:     3,
	WaitOnDependencies: true,
}

type deferringResource struct {
	id      string
	removes *int
}

func (r *deferringResource) Remove(_ context.Context) error {
	*r.removes++
	return liberrors.ErrDeferResource("blocked by an object scheduled for deletion")
}

func (r *deferringResource) String() string { return r.id }

type dependentResource struct {
	id      string
	removes *int
}

func (r *dependentResource) Remove(_ context.Context) error {
	*r.removes++
	return nil
}

func (r *dependentResource) String() string { return r.id }

type fixedLister struct{ resources []resource.Resource }

func (l *fixedLister) List(_ context.Context, _ interface{}) ([]resource.Resource, error) {
	return l.resources, nil
}

func newDeferTestNuke(t *testing.T, types ...string) *Nuke {
	t.Helper()
	n := New(testParametersDefer, nil, nil)
	n.SetLogger(logrus.WithField("test", true))
	n.SetRunSleep(time.Millisecond * 5)

	s, err := scanner.New(&scanner.Config{Owner: "Owner", ResourceTypes: types})
	assert.NoError(t, err)
	assert.NoError(t, n.RegisterScanner(testScope, s))
	return n
}

// A deferred resource ends the run successfully after one attempt, instead of being retried
// until the wait retries run out.
func Test_Nuke_Run_ItemStateDeferred(t *testing.T) {
	removes := 0
	registry.ClearRegistry()
	registry.Register(&registry.Registration{
		Name:   "DeferringResource",
		Scope:  testScope,
		Lister: &fixedLister{resources: []resource.Resource{&deferringResource{id: "ca-1", removes: &removes}}},
	})

	n := newDeferTestNuke(t, "DeferringResource")
	runErr := n.Run(context.TODO())

	assert.NoError(t, runErr)
	assert.Equal(t, 1, n.Queue.Count(queue.ItemStateDeferred))
	assert.Equal(t, 0, n.Queue.Count(queue.ItemStateFailed))
	assert.Equal(t, 1, removes, "a deferred resource is attempted once, not retried within the run")
}

// A resource whose dependency is deferred is deferred too and never attempted: the dependency
// is still standing, so removing its dependent would break the order DependsOn promises.
func Test_Nuke_Run_DeferredBlocksDependents(t *testing.T) {
	parentRemoves, childRemoves := 0, 0
	registry.ClearRegistry()
	registry.Register(&registry.Registration{
		Name:   "DeferringResource",
		Scope:  testScope,
		Lister: &fixedLister{resources: []resource.Resource{&deferringResource{id: "ca-1", removes: &parentRemoves}}},
	})
	registry.Register(&registry.Registration{
		Name:      "DependentResource",
		Scope:     testScope,
		DependsOn: []string{"DeferringResource"},
		Lister:    &fixedLister{resources: []resource.Resource{&dependentResource{id: "key-1", removes: &childRemoves}}},
	})

	n := newDeferTestNuke(t, "DeferringResource", "DependentResource")
	runErr := n.Run(context.TODO())

	assert.NoError(t, runErr)
	assert.Equal(t, 2, n.Queue.Count(queue.ItemStateDeferred))
	assert.Equal(t, 0, childRemoves, "the dependent of a deferred resource must not be removed")
	for _, item := range n.Queue.GetItems() {
		if item.Type == "DependentResource" {
			assert.Equal(t, "blocked by deferred DeferringResource ca-1", item.GetReason())
		}
	}
}

// A real failure alongside a deferred resource still fails the run: deferring is not a way
// to hide errors.
func Test_Nuke_Run_DeferredDoesNotMaskFailures(t *testing.T) {
	removes := 0
	registry.ClearRegistry()
	registry.Register(&registry.Registration{
		Name:   "DeferringResource",
		Scope:  testScope,
		Lister: &fixedLister{resources: []resource.Resource{&deferringResource{id: "ca-1", removes: &removes}}},
	})
	registry.Register(&registry.Registration{
		Name:   "FailingResource",
		Scope:  testScope,
		Lister: &fixedLister{resources: []resource.Resource{&failingResource{id: "broken-1"}}},
	})

	n := newDeferTestNuke(t, "DeferringResource", "FailingResource")
	runErr := n.Run(context.TODO())

	assert.Error(t, runErr)
	assert.Equal(t, 1, n.Queue.Count(queue.ItemStateDeferred))
	assert.Equal(t, 1, n.Queue.Count(queue.ItemStateFailed))
}

type failingResource struct{ id string }

func (r *failingResource) Remove(_ context.Context) error {
	return assert.AnError
}

func (r *failingResource) String() string { return r.id }

// plannedResource defers itself at scan time when told to, and counts the Remove calls that
// a planned deferral must prevent.
type plannedResource struct {
	id        string
	deferIt   bool
	deferErr  error
	schedules bool
	removes   *int
}

func (r *plannedResource) Defer(_ context.Context) (string, bool, error) {
	return "blocked by an object scheduled for deletion", r.deferIt, r.deferErr
}

func (r *plannedResource) SchedulesDeletion() bool { return r.schedules }

func (r *plannedResource) Remove(_ context.Context) error {
	*r.removes++
	return nil
}

func (r *plannedResource) String() string { return r.id }

// A resource that says at scan time it must wait is deferred in the plan and never attempted.
func Test_Nuke_Scan_DeferredIsNeverAttempted(t *testing.T) {
	removes := 0
	registry.ClearRegistry()
	registry.Register(&registry.Registration{
		Name:   "PlannedResource",
		Scope:  testScope,
		Lister: &fixedLister{resources: []resource.Resource{&plannedResource{id: "ca-1", deferIt: true, removes: &removes}}},
	})

	n := newDeferTestNuke(t, "PlannedResource")
	assert.NoError(t, n.Run(context.TODO()))
	assert.Equal(t, 1, n.Queue.Count(queue.ItemStateDeferred))
	assert.Equal(t, 0, removes, "a resource deferred at scan time must not be attempted")
}

// Deferral follows a DependsOn chain of any length in the plan, before anything is removed.
func Test_Nuke_Scan_DeferralFollowsTheDependencyChain(t *testing.T) {
	removes := 0
	registry.ClearRegistry()
	// Registered child first, so that one pass in scan order cannot resolve the chain.
	registry.Register(&registry.Registration{
		Name:      "Vault",
		Scope:     testScope,
		DependsOn: []string{"Key"},
		Lister:    &fixedLister{resources: []resource.Resource{&plannedResource{id: "vault-1", removes: &removes}}},
	})
	registry.Register(&registry.Registration{
		Name:      "Key",
		Scope:     testScope,
		DependsOn: []string{"CA"},
		Lister:    &fixedLister{resources: []resource.Resource{&plannedResource{id: "key-1", removes: &removes}}},
	})
	registry.Register(&registry.Registration{
		Name:   "CA",
		Scope:  testScope,
		Lister: &fixedLister{resources: []resource.Resource{&plannedResource{id: "ca-1", deferIt: true, removes: &removes}}},
	})

	n := newDeferTestNuke(t, "Vault", "Key", "CA")
	assert.NoError(t, n.Scan(context.TODO()))
	assert.Equal(t, 3, n.Queue.Count(queue.ItemStateDeferred))
	reasons := map[string]string{}
	for _, item := range n.Queue.GetItems() {
		reasons[item.Type] = item.GetReason()
	}
	assert.Equal(t, "blocked by deferred CA ca-1", reasons["Key"])
	assert.Equal(t, "blocked by deferred Key key-1", reasons["Vault"])
	assert.Equal(t, 0, removes)
}

// A failed check does not fail the scan or defer the resource: it is planned for removal.
func Test_Nuke_Scan_FailedDeferCheckPlansRemoval(t *testing.T) {
	removes := 0
	registry.ClearRegistry()
	registry.Register(&registry.Registration{
		Name:  "PlannedResource",
		Scope: testScope,
		Lister: &fixedLister{resources: []resource.Resource{
			&plannedResource{id: "ca-1", deferIt: true, deferErr: assert.AnError, removes: &removes},
		}},
	})

	n := newDeferTestNuke(t, "PlannedResource")
	assert.NoError(t, n.Scan(context.TODO()))
	assert.Equal(t, 0, n.Queue.Count(queue.ItemStateDeferred))
	assert.Equal(t, 1, n.Queue.Count(queue.ItemStateNew))
}

// A filtered resource is not asked whether it must be deferred, and stays filtered.
func Test_Nuke_Scan_FilteredIsNotDeferred(t *testing.T) {
	removes := 0
	registry.ClearRegistry()
	registry.Register(&registry.Registration{
		Name:   "PlannedResource",
		Scope:  testScope,
		Lister: &fixedLister{resources: []resource.Resource{&plannedResource{id: "ca-1", deferIt: true, removes: &removes}}},
	})

	n := newDeferTestNuke(t, "PlannedResource")
	n.Filters = filter.Filters{"PlannedResource": []filter.Filter{{Type: filter.Exact, Value: "ca-1"}}}
	assert.NoError(t, n.Scan(context.TODO()))
	assert.Equal(t, 1, n.Queue.Count(queue.ItemStateFiltered))
	assert.Equal(t, 0, n.Queue.Count(queue.ItemStateDeferred))
}

// A resource that only schedules its deletion is planned like any other, and known as such.
func Test_Nuke_Scan_SchedulableIsPlanned(t *testing.T) {
	removes := 0
	registry.ClearRegistry()
	registry.Register(&registry.Registration{
		Name:   "PlannedResource",
		Scope:  testScope,
		Lister: &fixedLister{resources: []resource.Resource{&plannedResource{id: "cert-1", schedules: true, removes: &removes}}},
	})

	n := newDeferTestNuke(t, "PlannedResource")
	assert.NoError(t, n.Scan(context.TODO()))
	item := n.Queue.GetItems()[0]
	assert.True(t, item.SchedulesDeletion())
	assert.Equal(t, queue.ItemStateNew, item.GetState())
}

// linkedResource names the resources it blocks, the way a CA names its signing key.
type linkedResource struct {
	id      string
	deferIt bool
	blocks  []resource.Ref
	removes map[string]int
}

func (r *linkedResource) Defer(_ context.Context) (string, bool, error) {
	return "blocked by its certificates", r.deferIt, nil
}

func (r *linkedResource) Blocks() []resource.Ref { return r.blocks }

func (r *linkedResource) Remove(_ context.Context) error {
	r.removes[r.id]++
	return nil
}

func (r *linkedResource) Properties() types.Properties {
	return types.NewProperties().Set("ID", r.id)
}

func (r *linkedResource) String() string { return r.id }

// A deferred resource that names what it blocks holds back only those resources: the key and
// vault behind the other, unblocked CA stay planned for removal.
func Test_Nuke_Scan_DeferralFollowsInstancesNotTypes(t *testing.T) {
	removes := map[string]int{}
	res := func(id string, deferIt bool, blocks ...resource.Ref) resource.Resource {
		return &linkedResource{id: id, deferIt: deferIt, blocks: blocks, removes: removes}
	}
	registry.ClearRegistry()
	registry.Register(&registry.Registration{
		Name:  "CA",
		Scope: testScope,
		Lister: &fixedLister{resources: []resource.Resource{
			res("ca-a", true, resource.Ref{Type: "Key", ID: "key-a"}),
			res("ca-b", false, resource.Ref{Type: "Key", ID: "key-b"}),
		}},
	})
	registry.Register(&registry.Registration{
		Name:      "Key",
		Scope:     testScope,
		DependsOn: []string{"CA"},
		Lister: &fixedLister{resources: []resource.Resource{
			res("key-a", false, resource.Ref{Type: "Vault", ID: "vault-a"}),
			res("key-b", false, resource.Ref{Type: "Vault", ID: "vault-b"}),
		}},
	})
	registry.Register(&registry.Registration{
		Name:      "Vault",
		Scope:     testScope,
		DependsOn: []string{"Key"},
		Lister:    &fixedLister{resources: []resource.Resource{res("vault-a", false), res("vault-b", false)}},
	})

	n := newDeferTestNuke(t, "CA", "Key", "Vault")
	assert.NoError(t, n.Scan(context.TODO()))
	states := map[string]queue.ItemState{}
	reasons := map[string]string{}
	for _, item := range n.Queue.GetItems() {
		id, _ := item.GetProperty("ID")
		states[id], reasons[id] = item.GetState(), item.GetReason()
	}
	assert.Equal(t, queue.ItemStateDeferred, states["key-a"])
	assert.Equal(t, "blocked by deferred CA ca-a", reasons["key-a"])
	assert.Equal(t, queue.ItemStateDeferred, states["vault-a"])
	assert.Equal(t, "blocked by deferred Key key-a", reasons["vault-a"])
	assert.Equal(t, queue.ItemStateNewDependency, states["key-b"], "key-b is not blocked by the deferred CA")
	assert.Equal(t, queue.ItemStateNewDependency, states["vault-b"])
}

// A filtered certificate keeps its CA, and the CA's key and vault in turn, even where the CA
// itself asked to be deferred: a deferral would promise a later run that can never take it.
func Test_Nuke_Scan_FilteredBlockerKeepsWhatItBlocks(t *testing.T) {
	removes := map[string]int{}
	res := func(id string, deferIt bool, blocks ...resource.Ref) resource.Resource {
		return &linkedResource{id: id, deferIt: deferIt, blocks: blocks, removes: removes}
	}
	registry.ClearRegistry()
	registry.Register(&registry.Registration{
		Name:   "Cert",
		Scope:  testScope,
		Lister: &fixedLister{resources: []resource.Resource{res("cert-a", false, resource.Ref{Type: "CA", ID: "ca-a"})}},
	})
	registry.Register(&registry.Registration{
		Name:      "CA",
		Scope:     testScope,
		DependsOn: []string{"Cert"},
		Lister: &fixedLister{resources: []resource.Resource{
			res("ca-a", true, resource.Ref{Type: "Key", ID: "key-a"}),
			res("ca-b", false, resource.Ref{Type: "Key", ID: "key-b"}),
		}},
	})
	registry.Register(&registry.Registration{
		Name:      "Key",
		Scope:     testScope,
		DependsOn: []string{"CA"},
		Lister: &fixedLister{resources: []resource.Resource{
			res("key-a", false, resource.Ref{Type: "Vault", ID: "vault-a"}),
			res("key-b", false),
		}},
	})
	registry.Register(&registry.Registration{
		Name:      "Vault",
		Scope:     testScope,
		DependsOn: []string{"Key"},
		Lister:    &fixedLister{resources: []resource.Resource{res("vault-a", false)}},
	})

	n := newDeferTestNuke(t, "Cert", "CA", "Key", "Vault")
	n.Filters = filter.Filters{"Cert": []filter.Filter{{Type: filter.Exact, Value: "cert-a"}}}
	assert.NoError(t, n.Scan(context.TODO()))
	states := map[string]queue.ItemState{}
	reasons := map[string]string{}
	for _, item := range n.Queue.GetItems() {
		id, _ := item.GetProperty("ID")
		states[id], reasons[id] = item.GetState(), item.GetReason()
	}
	assert.Equal(t, queue.ItemStateFiltered, states["ca-a"])
	assert.Equal(t, "blocked by filtered Cert cert-a", reasons["ca-a"])
	assert.Equal(t, queue.ItemStateFiltered, states["key-a"])
	assert.Equal(t, "blocked by filtered CA ca-a", reasons["key-a"])
	assert.Equal(t, queue.ItemStateFiltered, states["vault-a"])
	assert.Equal(t, queue.ItemStateNewDependency, states["ca-b"])
	assert.Equal(t, queue.ItemStateNewDependency, states["key-b"])
	assert.Equal(t, 0, n.Queue.Count(queue.ItemStateDeferred))
}

// A filtered resource that names nothing it blocks keeps nothing: DependsOn alone relates
// only types, and keeping every dependent of a filtered type would keep far too much.
func Test_Nuke_Scan_FilteredWithoutLinksKeepsNothing(t *testing.T) {
	removes := 0
	registry.ClearRegistry()
	registry.Register(&registry.Registration{
		Name:   "Parent",
		Scope:  testScope,
		Lister: &fixedLister{resources: []resource.Resource{&plannedResource{id: "parent-1", removes: &removes}}},
	})
	registry.Register(&registry.Registration{
		Name:      "Child",
		Scope:     testScope,
		DependsOn: []string{"Parent"},
		Lister:    &fixedLister{resources: []resource.Resource{&plannedResource{id: "child-1", removes: &removes}}},
	})

	n := newDeferTestNuke(t, "Parent", "Child")
	n.Filters = filter.Filters{"Parent": []filter.Filter{{Type: filter.Exact, Value: "parent-1"}}}
	assert.NoError(t, n.Scan(context.TODO()))
	assert.Equal(t, 1, n.Queue.Count(queue.ItemStateFiltered))
	assert.Equal(t, 1, n.Queue.Count(queue.ItemStateNewDependency))
}
