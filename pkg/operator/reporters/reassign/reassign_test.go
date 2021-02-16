package reassign

import (
	"context"
	"testing"
	"time"

	"github.com/mfojtik/bugzilla-operator/pkg/operator/bugutil"

	"github.com/eparis/bugzilla"

	"github.com/davecgh/go-spew/spew"

	"github.com/openshift/library-go/pkg/controller/factory"
	"github.com/openshift/library-go/pkg/operator/events"

	"github.com/mfojtik/bugzilla-operator/pkg/operator/config"

	"github.com/mfojtik/bugzilla-operator/pkg/cache"
	"github.com/mfojtik/bugzilla-operator/pkg/operator/controller"
)

func fakeBugClient(bugs map[int]bugzilla.Bug) func(bool) cache.BugzillaClient {
	return func(bool) cache.BugzillaClient {
		return &cache.FakeBugzillaClient{
			Fake: &bugzilla.Fake{
				Bugs: bugs,
			},
		}
	}
}

func fakeBugHistory(history map[int][]bugzilla.History) func(int, string) ([]bugzilla.History, error) {
	return func(id int, _ string) ([]bugzilla.History, error) {
		return history[id], nil
	}
}

func TestReassignReport(t *testing.T) {
	storage := []bug{
		{
			ID: 3,
			ComponentChanges: []componentChange{
				{
					From: "kube-apiserver",
					To:   "openshift-apiserver",
					When: bugutil.ParseChangeWhenString("2006-01-02T15:04:05Z"),
				},
				{
					From: "openshift-apiserver",
					To:   "etcd",
					When: bugutil.ParseChangeWhenString("2006-01-03T15:04:05Z"),
				},
			},
			LastComponentChange: time.Time{},
		},
	}

	r := &ReassignReporter{
		ControllerContext: controller.NewControllerContext(fakeBugClient(
			map[int]bugzilla.Bug{
				1: {ID: 1},
				2: {ID: 2},
				3: {ID: 3},
			},
		), nil, nil, nil),
		getBugHistory: fakeBugHistory(map[int][]bugzilla.History{
			// This bug should be added to storage
			1: {
				{
					When: "2006-01-02T15:04:05Z",
					Changes: []bugzilla.HistoryChange{
						{
							FieldName: "component",
							Removed:   "kube-apiserver",
							Added:     "Networking",
						},
					},
				},
				{
					When: "2006-01-03T15:04:05Z",
					Changes: []bugzilla.HistoryChange{
						{
							FieldName: "component",
							Removed:   "Networking",
							Added:     "installer",
						},
					},
				},
			},
			// This bug should be ignored as there is no interesting component here
			2: {
				{
					When: "2006-01-02T15:04:05Z",
					Changes: []bugzilla.HistoryChange{
						{
							FieldName: "component",
							Removed:   "openshift-apiserver",
							Added:     "etcd",
						},
					},
				},
				{
					When: "2006-01-03T15:04:05Z",
					Changes: []bugzilla.HistoryChange{
						{
							FieldName: "component",
							Removed:   "etcd",
							Added:     "Networking",
						},
					},
				},
			},
			// This bug should be updated in storage to include new transitions
			3: {
				{
					When: "2006-01-02T15:04:05Z",
					Changes: []bugzilla.HistoryChange{
						{
							FieldName: "component",
							Removed:   "kube-apiserver",
							Added:     "openshift-apiserver",
						},
					},
				},
				{
					When: "2006-01-03T15:04:05Z",
					Changes: []bugzilla.HistoryChange{
						{
							FieldName: "component",
							Removed:   "openshift-apiserver",
							Added:     "etcd",
						},
					},
				},
				{
					When: "2006-01-04T15:04:05Z",
					Changes: []bugzilla.HistoryChange{
						{
							FieldName: "component",
							Removed:   "etcd",
							Added:     "Networking",
						},
					},
				},
			},
		}),

		config: config.OperatorConfig{
			Components: map[string]config.Component{"kube-apiserver": {}},
		},
		writeReportFn: func(ctx context.Context, bugs []bug) error {
			storage = bugs
			return nil
		},
		readReportFn: func(ctx context.Context) ([]bug, error) {
			return storage, nil
		},
	}

	if err := r.sync(context.TODO(), factory.NewSyncContext("test", events.NewInMemoryRecorder("test"))); err != nil {
		t.Fatal(err)
	}

	for _, b := range storage {
		if b.ID == 2 {
			t.Errorf("bug without transitions in components of interest should not be reported (bug#2:\n%s\n)", spew.Sdump(storage))
		}
	}

	if len(storage) != 2 {
		t.Errorf("expected exactly 2 bugs be reported, got %d:\n%s\n", len(storage), spew.Sdump(storage))
	}

	for _, b := range storage {
		if b.ID != 3 {
			continue
		}
		if len(b.ComponentChanges) != 3 {
			t.Errorf("expected exactly 3 transitions reported for bug#3, got %d:\n%s\n", len(b.ComponentChanges), spew.Sdump(b.ComponentChanges))
		}
	}
}

func TestCounter(t *testing.T) {
	c := &componentCounter{}
	c.counts = append(c.counts, []componentCount{
		{
			name:  "c1",
			count: 5,
		},
		{
			name:  "c2",
			count: 1,
		},
		{
			name:  "c3",
			count: 10,
		},
	}...)
	c.Inc("c2")
	counts := c.byCount()

	if counts[0].name != "c3" {
		t.Errorf("expected first item to be c3 with highest count, got %v", counts[0])
	}
	if counts[2].name != "c2" && counts[2].count != 2 {
		t.Errorf("expected last item to be c2 with count==2, got %v", counts[2])
	}
}
