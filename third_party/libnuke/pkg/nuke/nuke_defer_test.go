package nuke

// entigo patch: tests for ErrDeferResource / ItemStateDeferred.

import (
	"context"
	"testing"
	"time"

	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"

	liberrors "github.com/ekristen/libnuke/pkg/errors"
	"github.com/ekristen/libnuke/pkg/queue"
	"github.com/ekristen/libnuke/pkg/registry"
	"github.com/ekristen/libnuke/pkg/resource"
	"github.com/ekristen/libnuke/pkg/scanner"
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
			assert.Equal(t, "blocked by deferred DeferringResource", item.GetReason())
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
