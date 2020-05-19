package blockers

import (
	"reflect"
	"strings"
	"testing"

	"github.com/eparis/bugzilla"

	"github.com/mfojtik/bugzilla-operator/pkg/operator/bugutil"
	"github.com/mfojtik/bugzilla-operator/pkg/operator/config"
)

func TestNewBlockersReporter_Triage(t *testing.T) {
	tests := []struct {
		name       string
		bugs       []bugzilla.Bug
		blockerIDs []int
		triageIDs  []int
		target     string
	}{
		{
			name:   "bug is blocker",
			target: "4.5.0",
			bugs: []bugzilla.Bug{
				{
					ID:            1,
					TargetRelease: []string{"4.5.0"},
					Severity:      "high",
				},
			},
			blockerIDs: []int{1},
			triageIDs:  []int{},
		},
		{
			name:   "bug target release is higher than target",
			target: "4.5.0",
			bugs: []bugzilla.Bug{
				{
					ID:            1,
					TargetRelease: []string{"4.6.0"},
					Severity:      "high",
				},
			},
			blockerIDs: []int{},
			triageIDs:  []int{},
		},
		{
			name:   "bug target release is lower than target",
			target: "4.5.0",
			bugs: []bugzilla.Bug{
				{
					ID:            1,
					TargetRelease: []string{"4.4.0"},
					Severity:      "high",
				},
			},
			blockerIDs: []int{},
			triageIDs:  []int{},
		},
		{
			name:   "bug target is not set and need triage",
			target: "4.5.0",
			bugs: []bugzilla.Bug{
				{
					ID:            1,
					TargetRelease: []string{"---"},
					Severity:      "high",
				},
			},
			blockerIDs: []int{},
			triageIDs:  []int{1},
		},
		{
			name:   "bug target and severity is not set and need triage",
			target: "4.5.0",
			bugs: []bugzilla.Bug{
				{
					ID:            1,
					TargetRelease: []string{"---"},
					Severity:      "unspecified",
				},
			},
			blockerIDs: []int{},
			triageIDs:  []int{1},
		},
		{
			name:   "bug severity is not set, but it is a blocker and need triage",
			target: "4.5.0",
			bugs: []bugzilla.Bug{
				{
					ID:            1,
					TargetRelease: []string{"4.5.0"},
					Severity:      "unspecified",
				},
			},
			blockerIDs: []int{1},
			triageIDs:  []int{1},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			r := &BlockersReporter{
				config: config.OperatorConfig{
					Release: config.BugzillaRelease{
						CurrentTargetRelease: test.target,
					},
				},
			}
			bugIDs := []int{}
			bugMap := map[int]bugzilla.Bug{}
			for _, b := range test.bugs {
				bugIDs = append(bugIDs, b.ID)
				bugMap[b.ID] = b
			}
			client := &bugzilla.Fake{
				Bugs: bugMap,
			}
			result := r.triageBug(client, bugIDs...)

			var expectedBlockers []string
			for _, b := range test.blockerIDs {
				bug, _ := client.GetBug(b)
				expectedBlockers = append(expectedBlockers, bugutil.FormatBugMessage(*bug))
			}

			var expectedTriage []string
			for _, b := range test.triageIDs {
				bug, _ := client.GetBug(b)
				expectedTriage = append(expectedTriage, bugutil.FormatBugMessage(*bug))
			}

			if !reflect.DeepEqual(result.blockers, expectedBlockers) {
				t.Errorf("expected:\n%s\n as blockers, got:\n%s", strings.Join(expectedBlockers, "\n"), strings.Join(result.blockers, "\n"))
			}
			if !reflect.DeepEqual(result.needTriage, expectedTriage) {
				t.Errorf("expected:\n%s\n need triage, got:\n%s", strings.Join(expectedTriage, "\n"), strings.Join(result.needTriage, "\n"))
			}
		})
	}
}
