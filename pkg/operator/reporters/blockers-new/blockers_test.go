package blockers

import (
	"testing"

	"k8s.io/apimachinery/pkg/util/sets"

	"github.com/eparis/bugzilla"
)

func TestNewBlockersReporter_Triage(t *testing.T) {
	tests := []struct {
		name                   string
		bugs                   []*bugzilla.Bug
		blockerIDs             []int
		blockerQuestionmarkIDs []int
		triageIDs              []int
		urgentIDs              []int
		target                 string
	}{
		{
			name:   "bug is blocker of current release",
			target: "4.5.0",
			bugs: []*bugzilla.Bug{
				{
					Status:        "POST",
					ID:            1,
					TargetRelease: []string{"4.5.0"},
					Severity:      "low",
					Priority:      "low",
					Flags: []bugzilla.Flag{
						{Name: "blocker", Status: "+"},
					},
				},
			},
			blockerIDs:             []int{1},
			blockerQuestionmarkIDs: []int{},
			triageIDs:              []int{},
			urgentIDs:              []int{},
		},
		{
			name:   "bug is high, but no blocker",
			target: "4.5.0",
			bugs: []*bugzilla.Bug{
				{
					Status:        "NEW",
					ID:            1,
					TargetRelease: []string{"4.5.0"},
					Severity:      "high",
					Priority:      "high",
				},
			},
			blockerIDs:             []int{},
			blockerQuestionmarkIDs: []int{},
			triageIDs:              []int{1},
			urgentIDs:              []int{},
		},
		{
			name:   "bug is high, no blocker+, but blocker?",
			target: "4.5.0",
			bugs: []*bugzilla.Bug{
				{
					Status:        "POST",
					ID:            1,
					TargetRelease: []string{"4.5.0"},
					Severity:      "high",
					Priority:      "high",
					Flags: []bugzilla.Flag{
						{Name: "blocker", Status: "?"},
					},
				},
			},
			blockerIDs:             []int{},
			blockerQuestionmarkIDs: []int{1},
			triageIDs:              []int{},
			urgentIDs:              []int{},
		},
		{
			name:   "bug is NEW, no blocker+, no blocker?",
			target: "4.5.0",
			bugs: []*bugzilla.Bug{
				{
					Status:        "NEW",
					ID:            1,
					TargetRelease: []string{"4.5.0"},
					Severity:      "low",
					Priority:      "low",
				},
			},
			blockerIDs:             []int{},
			blockerQuestionmarkIDs: []int{},
			triageIDs:              []int{1},
			urgentIDs:              []int{},
		},
		{
			name:   "bug target release is higher than target",
			target: "4.5.0",
			bugs: []*bugzilla.Bug{
				{
					ID:            1,
					TargetRelease: []string{"4.6.0"},
					Severity:      "low",
					Priority:      "low",
					Flags: []bugzilla.Flag{
						{Name: "blocker", Status: "+"},
					},
				},
			},
			blockerIDs:             []int{},
			blockerQuestionmarkIDs: []int{},
			triageIDs:              []int{},
			urgentIDs:              []int{},
		},
		{
			name:   "bug target release is lower than target",
			target: "4.5.0",
			bugs: []*bugzilla.Bug{
				{
					ID:            1,
					TargetRelease: []string{"4.4.0"},
					Severity:      "low",
					Priority:      "low",
					Flags: []bugzilla.Flag{
						{Name: "blocker", Status: "+"},
					},
				},
			},
			blockerIDs:             []int{},
			blockerQuestionmarkIDs: []int{},
			triageIDs:              []int{},
			urgentIDs:              []int{},
		},
		{
			name:   "bug target release is lower than target, urgent severity, but low priority",
			target: "4.5.0",
			bugs: []*bugzilla.Bug{
				{
					ID:            1,
					TargetRelease: []string{"4.3.0"},
					Severity:      "urgent",
					Priority:      "low",
				},
			},
			blockerIDs:             []int{},
			blockerQuestionmarkIDs: []int{},
			triageIDs:              []int{},
			urgentIDs:              []int{},
		},
		{
			name:   "bug target release is lower than target, urgent priority, low severity",
			target: "4.5.0",
			bugs: []*bugzilla.Bug{
				{
					ID:            1,
					TargetRelease: []string{"4.3.0"},
					Severity:      "low",
					Priority:      "urgent",
				},
			},
			blockerIDs:             []int{},
			blockerQuestionmarkIDs: []int{},
			triageIDs:              []int{},
			urgentIDs:              []int{1},
		},
		{
			name:   "bug target release is lower than target, urgent severity, unspecified priority",
			target: "4.5.0",
			bugs: []*bugzilla.Bug{
				{
					ID:            1,
					TargetRelease: []string{"4.3.0"},
					Severity:      "urgent",
					Priority:      "unspecified",
				},
			},
			blockerIDs:             []int{},
			blockerQuestionmarkIDs: []int{},
			triageIDs:              []int{1},
			urgentIDs:              []int{1},
		},
		{
			name:   "bug target is not set and need needTriage",
			target: "4.5.0",
			bugs: []*bugzilla.Bug{
				{
					ID:            1,
					TargetRelease: []string{"---"},
					Severity:      "high",
					Priority:      "high",
				},
			},
			blockerIDs:             []int{},
			blockerQuestionmarkIDs: []int{},
			triageIDs:              []int{1},
			urgentIDs:              []int{},
		},
		{
			name:   "bug target and severity is not set and need needTriage",
			target: "4.5.0",
			bugs: []*bugzilla.Bug{
				{
					ID:            1,
					TargetRelease: []string{"---"},
					Severity:      "unspecified",
					Priority:      "high",
				},
			},
			blockerIDs:             []int{},
			blockerQuestionmarkIDs: []int{},
			triageIDs:              []int{1},
			urgentIDs:              []int{},
		},
		{
			name:   "bug priority is not set and need needTriage",
			target: "4.5.0",
			bugs: []*bugzilla.Bug{
				{
					ID:            1,
					TargetRelease: []string{"4.6.0"},
					Severity:      "high",
					Priority:      "unspecified",
				},
			},
			blockerIDs:             []int{},
			blockerQuestionmarkIDs: []int{},
			triageIDs:              []int{1},
			urgentIDs:              []int{},
		},
		{
			name:   "bug severity is not set, but it is a blocker and need needTriage",
			target: "4.5.0",
			bugs: []*bugzilla.Bug{
				{
					ID:            1,
					TargetRelease: []string{"4.5.0"},
					Severity:      "unspecified",
					Priority:      "high",
					Flags: []bugzilla.Flag{
						{Name: "blocker", Status: "+"},
					},
				},
			},
			blockerIDs:             []int{1},
			blockerQuestionmarkIDs: []int{},
			triageIDs:              []int{1},
			urgentIDs:              []int{},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var bugIDs []int
			bugMap := map[int]bugzilla.Bug{}
			for _, b := range test.bugs {
				bugIDs = append(bugIDs, b.ID)
				bugMap[b.ID] = *b
			}
			result := summarizeBugs(test.target, test.bugs...)

			if expected, got := sets.NewInt(test.blockerIDs...), sets.NewInt(result.blockerPlusIDs...); !expected.Equal(got) {
				t.Errorf("expected blocker bugs %v, got %v", expected.List(), got.List())
			}

			if expected, got := sets.NewInt(test.blockerQuestionmarkIDs...), sets.NewInt(result.blockerQuestionmarkIDs...); !expected.Equal(got) {
				t.Errorf("expected blocker? bugs %v, got %v", expected.List(), got.List())
			}

			if expected, got := sets.NewInt(test.triageIDs...), sets.NewInt(result.toTriageIDs...); !expected.Equal(got) {
				t.Errorf("expected to-triage bugs %v, got %v", expected.List(), got.List())
			}

			if expected, got := sets.NewInt(test.urgentIDs...), sets.NewInt(result.urgentIDs...); !expected.Equal(got) {
				t.Errorf("expected urgent bugs %v, got %v", expected.List(), got.List())
			}
		})
	}
}
