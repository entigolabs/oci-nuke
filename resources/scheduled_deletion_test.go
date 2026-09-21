package resources

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"testing"
	"time"

	liberrors "github.com/ekristen/libnuke/pkg/errors"
	"github.com/oracle/oci-go-sdk/v65/common"
)

// fakeService stands in for one of the four scheduling APIs: it holds a lifecycle state
// and a deletion date, and can be told to keep a date of its own choosing rather than the
// one it is given - which is what the certificates service was seen doing.
type fakeService struct {
	state      string
	scheduled  *time.Time
	keeps      *time.Duration // when set, the date the service records whatever it is asked for
	transition int            // reads to serve as CANCELLING_DELETION before settling
	schedules  int
	cancels    int
}

func (f *fakeService) schedule() deletionSchedule {
	return deletionSchedule{
		minRetention: 24*time.Hour + scheduledDeletionSkew,
		read: func(ctx context.Context) (string, *common.SDKTime, error) {
			if f.transition > 0 {
				f.transition--
				return stateCancellingDeletion, nil, nil
			}
			if f.scheduled == nil {
				return f.state, nil, nil
			}
			return f.state, &common.SDKTime{Time: *f.scheduled}, nil
		},
		schedule: func(ctx context.Context, at time.Time) error {
			f.schedules++
			if f.keeps != nil {
				at = time.Now().Add(*f.keeps)
			}
			f.state, f.scheduled = statePendingDeletion, &at
			return nil
		},
		cancel: func(ctx context.Context) error {
			f.cancels++
			f.state, f.scheduled = "ACTIVE", nil
			return nil
		},
	}
}

func TestEnsureSchedulesAnUntouchedResourceOnce(t *testing.T) {
	f := &fakeService{state: "ACTIVE"}
	if err := f.schedule().ensure(context.Background()); err != nil {
		t.Fatal(err)
	}
	if f.schedules != 1 || f.cancels != 0 {
		t.Fatalf("schedules=%d cancels=%d, want one schedule and no cancel", f.schedules, f.cancels)
	}
	if out := time.Until(*f.scheduled); out > 25*time.Hour {
		t.Fatalf("scheduled %s out, want the minimum", out)
	}
}

func TestEnsureLeavesAScheduleThatIsAlreadyEarlyEnough(t *testing.T) {
	at := time.Now().Add(24*time.Hour + 10*time.Minute)
	f := &fakeService{state: statePendingDeletion, scheduled: &at}
	if err := f.schedule().ensure(context.Background()); err != nil {
		t.Fatal(err)
	}
	if f.schedules != 0 || f.cancels != 0 {
		t.Fatalf("schedules=%d cancels=%d, want an early schedule left alone", f.schedules, f.cancels)
	}
}

func TestEnsurePullsInALongerSchedule(t *testing.T) {
	at := time.Now().Add(10 * 24 * time.Hour)
	f := &fakeService{state: statePendingDeletion, scheduled: &at}
	if err := f.schedule().ensure(context.Background()); err != nil {
		t.Fatal(err)
	}
	if f.cancels != 1 || f.schedules != 1 {
		t.Fatalf("cancels=%d schedules=%d, want the ten-day date withdrawn and re-issued", f.cancels, f.schedules)
	}
	if out := time.Until(*f.scheduled); out > 25*time.Hour {
		t.Fatalf("still scheduled %s out", out)
	}
}

func TestEnsureReportsADateTheServiceWillNotGiveUp(t *testing.T) {
	keeps := 7*24*time.Hour + 12*time.Hour
	f := &fakeService{state: "ACTIVE", keeps: &keeps}
	err := f.schedule().ensure(context.Background())
	if err == nil {
		t.Fatal("want an error naming the date the service kept, got nil")
	}
	t.Log(err)
}

// The services read the fractional part of a timestamp as milliseconds however many digits
// it has, and add it to the whole-second value, so a date carrying time.Now()'s nanoseconds
// is recorded hours later than it was asked for. ensure must send a date the service can
// round-trip; see deletionPrecision.
func TestEnsureSendsADateTheServiceCanRoundTrip(t *testing.T) {
	f := &fakeService{state: "ACTIVE"}
	d := f.schedule()

	var sent time.Time
	inner := d.schedule
	d.schedule = func(ctx context.Context, at time.Time) error {
		sent = at
		// What the service does: read the fraction's digits as milliseconds, whatever
		// their length, and add them to the whole-second value.
		return inner(ctx, at.Truncate(time.Second).Add(fractionAsMilliseconds(at)))
	}

	if err := d.ensure(context.Background()); err != nil {
		t.Fatal(err)
	}

	if sent.Nanosecond()%int(time.Millisecond) != 0 {
		t.Fatalf("sent %s, want no precision finer than a millisecond", sent.Format(time.RFC3339Nano))
	}
	if out := time.Until(*f.scheduled); out > 25*time.Hour {
		t.Fatalf("service recorded %s out, want the minimum - the fraction was misread", out)
	}
}

// A service refusing because dependents still exist is not a failure: they are themselves
// already scheduled, so the block clears without anyone doing anything. ensure reports it as
// a hold, which libnuke keeps out of the failure count.
func TestEnsureHoldsRatherThanFailsWhenDependentsStillExist(t *testing.T) {
	f := &fakeService{state: "ACTIVE"}
	d := f.schedule()
	d.schedule = func(ctx context.Context, at time.Time) error {
		return fmt.Errorf("cannot be scheduled for deletion because subordinate CAs or certificates exist")
	}
	d.blockedBy = func(err error) (string, bool) {
		if strings.Contains(err.Error(), "certificates exist") {
			return "certificates are still within their own retention", true
		}
		return "", false
	}

	err := d.ensure(context.Background())

	var hold liberrors.ErrHoldResource
	if !errors.As(err, &hold) {
		t.Fatalf("got %T (%v), want ErrHoldResource so libnuke reports it as hold", err, err)
	}
	if !strings.Contains(hold.Error(), "retention") {
		t.Fatalf("hold reason %q does not say why", hold.Error())
	}
}

// Anything the predicate does not recognise is still a failure.
func TestEnsureStillFailsOnAnUnrecognisedError(t *testing.T) {
	f := &fakeService{state: "ACTIVE"}
	d := f.schedule()
	d.schedule = func(ctx context.Context, at time.Time) error {
		return fmt.Errorf("500 InternalServerError")
	}
	d.blockedBy = func(err error) (string, bool) { return "", false }

	err := d.ensure(context.Background())

	var hold liberrors.ErrHoldResource
	if errors.As(err, &hold) {
		t.Fatal("an unrecognised error was reported as a hold")
	}
	if err == nil {
		t.Fatal("want an error")
	}
}

func TestEnsureWaitsOutATransitionRatherThanSchedulingDuringOne(t *testing.T) {
	f := &fakeService{state: "ACTIVE", transition: 1}
	if err := f.schedule().ensure(context.Background()); err != nil {
		t.Fatal(err)
	}
	if f.schedules != 1 {
		t.Fatalf("schedules=%d, want the schedule held until the resource settled", f.schedules)
	}
}

func TestScheduledLaterThanNecessary(t *testing.T) {
	min := 24*time.Hour + scheduledDeletionSkew
	for _, tc := range []struct {
		name string
		at   *common.SDKTime
		want bool
	}{
		{"no date at all", nil, true},
		{"our own schedule", &common.SDKTime{Time: time.Now().Add(min)}, false},
		{"our schedule seen later in the run", &common.SDKTime{Time: time.Now().Add(min - 30*time.Minute)}, false},
		{"the ten-day default", &common.SDKTime{Time: time.Now().Add(10 * 24 * time.Hour)}, true},
	} {
		if got := scheduledLaterThanNecessary(tc.at, min); got != tc.want {
			t.Errorf("%s: got %t, want %t", tc.name, got, tc.want)
		}
	}
}

// fractionAsMilliseconds reads the fractional digits of a timestamp the way the services
// do: as a count of milliseconds, however many digits there are.
func fractionAsMilliseconds(at time.Time) time.Duration {
	s := at.UTC().Format(time.RFC3339Nano)
	i := strings.IndexByte(s, '.')
	if i < 0 {
		return 0
	}
	j := i + 1
	for j < len(s) && s[j] >= '0' && s[j] <= '9' {
		j++
	}
	n, err := strconv.Atoi(s[i+1 : j])
	if err != nil {
		return 0
	}
	return time.Duration(n) * time.Millisecond
}
