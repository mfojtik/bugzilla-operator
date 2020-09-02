package reassigncontroller

import (
	"context"
	"reflect"
	"testing"

	"github.com/mfojtik/bugzilla-operator/pkg/cache"
	"github.com/mfojtik/bugzilla-operator/pkg/operator/config"
	"github.com/mfojtik/bugzilla-operator/pkg/operator/controller"
	"github.com/openshift/library-go/pkg/controller/factory"
	"github.com/openshift/library-go/pkg/operator/events"

	"github.com/eparis/bugzilla"
)

type testBugzillaClient struct {
	bugs            []*bugzilla.Bug
	bugsWithHistory map[int][]bugzilla.History
	bugUpdates      []bugzilla.BugUpdate
}

func (t *testBugzillaClient) Endpoint() string {
	panic("implement me")
}

func (t *testBugzillaClient) GetBug(id int) (*bugzilla.Bug, error) {
	panic("implement me")
}

func (t *testBugzillaClient) GetBugComments(id int) ([]bugzilla.Comment, error) {
	panic("implement me")
}

func (t *testBugzillaClient) GetBugHistory(id int) ([]bugzilla.History, error) {
	panic("implement me")
}

func (t *testBugzillaClient) Search(query bugzilla.Query) ([]*bugzilla.Bug, error) {
	return t.bugs, nil
}

func (t *testBugzillaClient) GetExternalBugPRsOnBug(id int) ([]bugzilla.ExternalBug, error) {
	panic("implement me")
}

func (t *testBugzillaClient) UpdateBug(id int, update bugzilla.BugUpdate) error {
	t.bugUpdates = append(t.bugUpdates, update)
	return nil
}

func (t *testBugzillaClient) AddPullRequestAsExternalBug(id int, org, repo string, num int) (bool, error) {
	panic("implement me")
}

func (t *testBugzillaClient) WithCGIClient(user, password string) bugzilla.Client {
	panic("implement me")
}

func (t *testBugzillaClient) BugList(queryName, sharerID string) ([]bugzilla.Bug, error) {
	panic("implement me")
}

func (t *testBugzillaClient) GetCachedBug(id int, lastChangedTime string) (*bugzilla.Bug, error) {
	panic("implement me")
}

func (t *testBugzillaClient) GetCachedBugComments(id int, lastChangedTime string) ([]bugzilla.Comment, error) {
	panic("implement me")
}

func (t *testBugzillaClient) GetCachedBugHistory(id int, lastChangedTime string) ([]bugzilla.History, error) {
	return t.bugsWithHistory[id], nil
}

func TestNewReassignController(t *testing.T) {
	tests := []struct {
		name            string
		history         []bugzilla.History
		expectedUpdated []bugzilla.BugUpdate
	}{
		{
			name: "component changed after we notified assignee",
			history: []bugzilla.History{
				{
					When: "2020-01-01T12:00:00Z",
					Changes: []bugzilla.HistoryChange{
						{
							FieldName: "cf_devel_whiteboard",
						},
					},
				},
				{
					When: "2020-01-01T12:05:00Z",
					Changes: []bugzilla.HistoryChange{
						{
							FieldName: "component",
						},
					},
				},
			},
			expectedUpdated: []bugzilla.BugUpdate{
				{
					DevWhiteboard: "",
					MinorUpdate:   true,
				},
			},
		},
		{
			name: "component never changed",
			history: []bugzilla.History{
				{
					When: "2020-01-01T12:00:00Z",
					Changes: []bugzilla.HistoryChange{
						{
							FieldName: "cf_devel_whiteboard",
						},
					},
				},
			},
			expectedUpdated: []bugzilla.BugUpdate{},
		},
		{
			name: "component changed multiple times but we catched up",
			history: []bugzilla.History{
				{
					When: "2020-01-01T12:00:00Z",
					Changes: []bugzilla.HistoryChange{
						{
							FieldName: "cf_devel_whiteboard",
						},
					},
				},
				{
					When: "2020-01-01T12:05:00Z",
					Changes: []bugzilla.HistoryChange{
						{
							FieldName: "component",
						},
					},
				},
				{
					When: "2020-01-01T12:10:00Z",
					Changes: []bugzilla.HistoryChange{
						{
							FieldName: "component",
						},
					},
				},
				{
					When: "2020-01-01T12:15:00Z",
					Changes: []bugzilla.HistoryChange{
						{
							FieldName: "cf_devel_whiteboard",
						},
					},
				},
			},
			expectedUpdated: []bugzilla.BugUpdate{},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			testClient := &testBugzillaClient{
				bugs: []*bugzilla.Bug{{
					ID: 1,
				}},
				bugsWithHistory: map[int][]bugzilla.History{
					1: test.history,
				},
			}
			ctx := controller.NewControllerContext(func(debug bool) cache.BugzillaClient {
				return testClient
			}, nil, nil, nil)
			c := &ReassignController{
				context: ctx,
				config:  &config.OperatorConfig{Components: map[string]config.Component{}},
			}
			if err := c.sync(context.TODO(), factory.NewSyncContext("test", events.NewInMemoryRecorder("test"))); err != nil {
				t.Errorf("unexpected error: %v", err)
			}
			if testClient.bugUpdates == nil {
				testClient.bugUpdates = []bugzilla.BugUpdate{}
			}
			if !reflect.DeepEqual(testClient.bugUpdates, test.expectedUpdated) {
				t.Errorf("expected:\n%#v\ngot:\n%#v", test.expectedUpdated, testClient.bugUpdates)
			}
		})
	}

}
