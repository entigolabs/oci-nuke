package resources

import (
	"context"
	"fmt"
	"time"

	liberrors "github.com/ekristen/libnuke/pkg/errors"
	"github.com/oracle/oci-go-sdk/v65/common"
)

// Nothing in OCI's crypto stack deletes on request. A certificate, CA, vault or key is
// only ever *scheduled*, never sooner than a day (certificates) or a week (everything
// else) out, and it then sits in PENDING_DELETION until that date arrives. Two things
// follow from that for all four resources, so they live here rather than four times over.
//
// The date has to be read back rather than assumed. Scheduling returns 200 without
// promising that the date it was handed is the date it kept: a certificate asked for
// 24h05m in eu-frankfurt-1 came back holding a date seven and a half days out, and a
// certificate scheduled with no date at all comes back ten days out rather than the
// thirty the API documents. So ensure() reads the state, acts on it, and reads it again -
// cancelling and re-issuing a schedule that sits later than what was asked for, and
// reporting the date the service actually kept when it will not take ours.
//
// The resource also has to stop being listed once its date is right, or the run never
// ends. libnuke only marks an item finished when the lister stops returning it, and
// nobody is going to hold a nuke open for the day or the week until the deletion is due -
// "waiting for removal" here means waiting for a date, not for an API call. Each lister
// therefore keeps a certificate, CA, vault or key out of its results once the schedule is
// within scheduledDeletionSlack of what this tool would ask for, and still returns one
// whose date is further out than that: the ten-day default, a date left by a terraform
// destroy, or one from a build of this tool that did not send a date at all.
const (
	// scheduledDeletionSkew is what is added to each service's documented minimum. For
	// the certificates service it is the service's own number - it rejects anything
	// "less than minimum 1440 even after allowable clock skew 5" - and it doubles as
	// cover for the flight time between reading the clock here and the service reading
	// its own.
	scheduledDeletionSkew = 5 * time.Minute

	// scheduledDeletionSlack is how much later than our own target an existing schedule
	// may sit before it is worth cancelling and re-issuing. It only has to cover the
	// drift between a resource scheduled at the start of a run and the same resource
	// listed again later in that run, both measured from "now".
	scheduledDeletionSlack = time.Hour

	// A schedule or a cancel is asynchronous: the resource passes through
	// SCHEDULING_DELETION or CANCELLING_DELETION first, so ensure() waits the transition
	// out rather than acting during one - seconds in practice.
	scheduledDeletionSettle = 90 * time.Second
	scheduledDeletionPoll   = 3 * time.Second

	// Each attempt performs one action - schedule, or cancel a schedule that is too far
	// out - and re-reads the result, so a handful covers cancel/re-schedule twice over.
	scheduledDeletionAttempts = 6
)

// Lifecycle states as the services spell them. Certificates, CAs, vaults and keys each
// have their own enum type with these same string values.
const (
	stateCreating           = "CREATING"
	stateUpdating           = "UPDATING"
	stateSchedulingDeletion = "SCHEDULING_DELETION"
	stateCancellingDeletion = "CANCELLING_DELETION"
	statePendingDeletion    = "PENDING_DELETION"
	stateDeleting           = "DELETING"
	stateDeleted            = "DELETED"
)

// deletionPrecision is the only fractional-second precision the services round-trip. They
// read the fraction as milliseconds however many digits it has and add it to the whole-second
// value, so a nanosecond timestamp is recorded hours later than it was asked for. Dropping
// the fraction is not the alternative: a whole-second timestamp is rejected outright with
// "Unable to process JSON input".
const deletionPrecision = time.Millisecond

// deletionSchedule drives one resource's deletion date through the service's three calls.
type deletionSchedule struct {
	// minRetention is the earliest the service accepts, counted from now.
	minRetention time.Duration
	read         func(ctx context.Context) (state string, scheduled *common.SDKTime, err error)
	schedule     func(ctx context.Context, at time.Time) error
	cancel       func(ctx context.Context) error

	// blockedBy reports whether an error from schedule means the service is refusing
	// because dependents of this resource still exist, and says so in a form fit to show
	// a human. That is not a failure: those dependents are themselves already scheduled,
	// so the block clears on its own and the answer is to run again later, not to fix
	// anything. ensure turns it into liberrors.ErrHoldResource, which libnuke reports as
	// `hold` with the reason rather than counting it among the failures.
	//
	// Only the recognition is per-service - each words its refusal differently - so this
	// closure sits beside read/schedule/cancel for the same reason they do. Nil where a
	// service has no such dependency.
	blockedBy func(err error) (reason string, ok bool)
}

// ensure leaves the resource scheduled for deletion at the earliest date the service
// accepts, whether it was untouched, already scheduled too far out, or mid-transition.
func (d deletionSchedule) ensure(ctx context.Context) error {
	var kept *common.SDKTime

	for attempt := 0; attempt < scheduledDeletionAttempts; attempt++ {
		state, scheduled, err := d.settled(ctx)
		if err != nil {
			return err
		}

		switch state {
		case stateDeleting, stateDeleted:
			return nil
		case statePendingDeletion:
			if !scheduledLaterThanNecessary(scheduled, d.minRetention) {
				return nil
			}
			// The date cannot be moved in place, only withdrawn and re-issued.
			kept = scheduled
			if err := d.cancel(ctx); err != nil {
				return err
			}
		default:
			if err := d.schedule(ctx, time.Now().Add(d.minRetention).UTC().Truncate(deletionPrecision)); err != nil {
				if d.blockedBy != nil {
					if reason, ok := d.blockedBy(err); ok {
						return liberrors.ErrHoldResource(reason)
					}
				}
				return err
			}
		}
	}

	return fmt.Errorf("OCI keeps the deletion at %s rather than the %s it was asked for; "+
		"cancelling and re-scheduling it %d times did not move it",
		formatDeletionTime(kept), time.Now().Add(d.minRetention).UTC().Format(time.RFC3339),
		scheduledDeletionAttempts)
}

// settled reads the resource, waiting out any transition first so that the caller never
// acts on a resource in the middle of one.
func (d deletionSchedule) settled(ctx context.Context) (string, *common.SDKTime, error) {
	deadline := time.Now().Add(scheduledDeletionSettle)

	for {
		state, scheduled, err := d.read(ctx)
		if err != nil {
			return "", nil, err
		}

		switch state {
		case stateCreating, stateUpdating, stateSchedulingDeletion, stateCancellingDeletion:
		default:
			return state, scheduled, nil
		}

		if time.Now().After(deadline) {
			// Left for libnuke to retry rather than acted on: this is the state a
			// schedule request should not be sent in.
			return "", nil, fmt.Errorf("still %s after %s", state, scheduledDeletionSettle)
		}

		select {
		case <-ctx.Done():
			return "", nil, ctx.Err()
		case <-time.After(scheduledDeletionPoll):
		}
	}
}

// scheduledLaterThanNecessary reports whether an existing deletion date is far enough out
// to be worth replacing. A resource with no date at all counts as needing one.
func scheduledLaterThanNecessary(scheduled *common.SDKTime, minRetention time.Duration) bool {
	if scheduled == nil {
		return true
	}
	return scheduled.After(time.Now().Add(minRetention + scheduledDeletionSlack))
}

func formatDeletionTime(scheduled *common.SDKTime) string {
	if scheduled == nil {
		return "an unreported time"
	}
	return scheduled.UTC().Format(time.RFC3339)
}
