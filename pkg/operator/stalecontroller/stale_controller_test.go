package stalecontroller

import (
	"reflect"
	"testing"

	"github.com/davecgh/go-spew/spew"
	"github.com/eparis/bugzilla"

	"github.com/mfojtik/bugzilla-operator/pkg/operator/config"
)

func TestNewStaleController(t *testing.T) {
	tests := []struct {
		name         string
		bug          bugzilla.Bug
		staleConfig  config.BugAction
		expectUpdate *bugzilla.BugUpdate
	}{
		{
			name: "add comment",
			bug: bugzilla.Bug{
				ID: 1,
			},
			staleConfig: config.BugAction{
				AddComment: "test comment",
			},
			expectUpdate: &bugzilla.BugUpdate{
				Status:        "",
				Resolution:    "",
				TargetRelease: "",
				DevWhiteboard: "",
				Comment: &bugzilla.BugComment{
					Body:     "test comment",
					Private:  false,
					Markdown: false,
				},
				Keywords: nil,
				Flags:    nil,
				Priority: "",
				Severity: "",
			},
		},
		{
			name: "add keyword",
			bug: bugzilla.Bug{
				ID: 1,
			},
			staleConfig: config.BugAction{
				AddKeyword: "test keyword",
			},
			expectUpdate: &bugzilla.BugUpdate{
				Status:        "",
				Resolution:    "",
				TargetRelease: "",
				DevWhiteboard: "test keyword",
				Comment:       nil,
				Keywords:      nil,
				Flags:         nil,
				Priority:      "",
				Severity:      "",
			},
		},
		{
			name: "add reporter needinfo",
			bug: bugzilla.Bug{
				ID:      1,
				Creator: "Clayton",
			},
			staleConfig: config.BugAction{
				NeedInfoFromCreator: true,
			},
			expectUpdate: &bugzilla.BugUpdate{
				Status:        "",
				Resolution:    "",
				TargetRelease: "",
				DevWhiteboard: "",
				Comment:       nil,
				Keywords:      nil,
				Flags: []bugzilla.FlagChange{{
					Name:      "needinfo",
					Status:    "?",
					Requestee: "Clayton",
				}},
				Priority: "",
				Severity: "",
			},
		},
		{
			name: "add assignee needinfo",
			bug: bugzilla.Bug{
				ID:         1,
				AssignedTo: "Clayton",
			},
			staleConfig: config.BugAction{
				NeedInfoFromAssignee: true,
			},
			expectUpdate: &bugzilla.BugUpdate{
				Status:        "",
				Resolution:    "",
				TargetRelease: "",
				DevWhiteboard: "",
				Comment:       nil,
				Keywords:      nil,
				Flags: []bugzilla.FlagChange{{
					Name:      "needinfo",
					Status:    "?",
					Requestee: "Clayton",
				}},
				Priority: "",
				Severity: "",
			},
		},
		{
			name: "all actions",
			bug: bugzilla.Bug{
				ID:         1,
				AssignedTo: "Clayton",
				Creator:    "David",
				Severity:   "high",
				Priority:   "high",
			},
			staleConfig: config.BugAction{
				AddComment: "comment",
				AddKeyword: "keyword",
				PriorityTransitions: []config.Transition{{
					From: "high",
					To:   "medium",
				}},
				SeverityTransitions: []config.Transition{{
					From: "high",
					To:   "medium",
				}},
				NeedInfoFromCreator:  true,
				NeedInfoFromAssignee: true,
			},
			expectUpdate: &bugzilla.BugUpdate{
				Status:        "",
				Resolution:    "",
				TargetRelease: "",
				DevWhiteboard: "keyword",
				Comment: &bugzilla.BugComment{
					Body: "comment",
				},
				Flags: []bugzilla.FlagChange{{
					Name:      "needinfo",
					Status:    "?",
					Requestee: "David,Clayton",
				}},
				Priority: "medium",
				Severity: "medium",
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			client := &bugzilla.Fake{
				Bugs: map[int]bugzilla.Bug{
					test.bug.ID: test.bug,
				},
			}
			c := &StaleController{
				config: config.OperatorConfig{Lists: config.BugzillaLists{
					Stale: config.BugzillaList{
						Action: test.staleConfig,
					},
				}},
			}
			gotUpdate, _, err := c.handleBug(client, test.bug)
			if err != nil {
				t.Errorf("got unexpected error: %v", err)
			}
			if !reflect.DeepEqual(gotUpdate, test.expectUpdate) {
				t.Errorf("expected:\n%s\ngot:\n%s\n", spew.Sdump(test.expectUpdate), spew.Sdump(gotUpdate))
			}
		})
	}
}
