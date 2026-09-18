package resources

import (
	"context"
	"testing"
	"time"

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
