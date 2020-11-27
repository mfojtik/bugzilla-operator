package blockers

import (
	"fmt"
	"net/url"
	"strings"

	"github.com/eparis/bugzilla"
	"k8s.io/apimachinery/pkg/util/sets"

	"github.com/mfojtik/bugzilla-operator/pkg/operator/bugutil"
)

var (
	seriousKeywords = []string{
		"ServiceDeliveryBlocker",
		"TestBlocker",
		"UpgradeBlocker",
	}
)

type bugSummary struct {
	seriousIDs             map[string][]int
	blockerPlus            []string
	blockerPlusIDs         []int
	blockerQuestionmark    []string
	blockerQuestionmarkIDs []int
	toTriage               []string
	toTriageIDs            []int
	needUpcomingSprint     []string
	needUpcomingSprintIDs  []int
	urgentIDs              []int
	urgent                 []string
	staleCount             int
	priorityCount          map[string]int
	severityCount          map[string]int
	currentReleaseCount    int
}

func summarizeBugs(currentTargetRelease string, bugs ...*bugzilla.Bug) bugSummary {
	r := bugSummary{
		priorityCount: map[string]int{},
		severityCount: map[string]int{},
		seriousIDs:    map[string][]int{},
	}
	for _, bug := range bugs {
		keywords := sets.NewString(bug.Keywords...)
		for _, keyword := range seriousKeywords {
			if keywords.Has(keyword) {
				r.seriousIDs[keyword] = append(r.seriousIDs[keyword], bug.ID)
			}
		}

		if strings.Contains(bug.Whiteboard, "LifecycleStale") {
			r.staleCount++
		}

		r.severityCount[bug.Severity]++
		r.priorityCount[bug.Priority]++

		if bug.Priority == "urgent" || (bug.Severity == "urgent" && bug.Priority == "unspecified") {
			r.urgent = append(r.urgent, bugutil.FormatBugMessage(*bug))
			r.urgentIDs = append(r.urgentIDs, bug.ID)
		}

		if !keywords.Has("UpcomingSprint") {
			r.needUpcomingSprint = append(r.needUpcomingSprint, bugutil.FormatBugMessage(*bug))
			r.needUpcomingSprintIDs = append(r.needUpcomingSprintIDs, bug.ID)
		}

		targetRelease := "---"
		if len(bug.TargetRelease) > 0 {
			targetRelease = bug.TargetRelease[0]
		}

		if hasFlag(bug, "blocker", "+") && (targetRelease == currentTargetRelease || targetRelease == "---") {
			r.blockerPlus = append(r.blockerPlus, bugutil.FormatBugMessage(*bug))
			r.blockerPlusIDs = append(r.blockerPlusIDs, bug.ID)
			r.seriousIDs["blocker+"] = append(r.seriousIDs["blocker+"], bug.ID)
		}

		if hasFlag(bug, "blocker", "?") && (targetRelease == currentTargetRelease || targetRelease == "---") {
			r.blockerQuestionmark = append(r.blockerQuestionmark, bugutil.FormatBugMessage(*bug))
			r.blockerQuestionmarkIDs = append(r.blockerQuestionmarkIDs, bug.ID)
			r.seriousIDs["blocker?"] = append(r.seriousIDs["blocker?"], bug.ID)
		}

		triageState := sets.NewString("NEW", "")
		if (targetRelease == currentTargetRelease && triageState.Has(bug.Status)) || targetRelease == "---" || bug.Priority == "unspecified" || bug.Priority == "" || bug.Severity == "unspecified" || bug.Severity == "" {
			r.toTriage = append(r.toTriage, bugutil.FormatBugMessage(*bug))
			r.toTriageIDs = append(r.toTriageIDs, bug.ID)
		}

		if targetRelease == currentTargetRelease || targetRelease == "---" {
			r.currentReleaseCount++
		}
	}

	return r
}

func hasFlag(bug *bugzilla.Bug, name, value string) bool {
	for _, f := range bug.Flags {
		if f.Name == name && f.Status == value {
			return true
		}
	}
	return false
}

func makeBugzillaLink(hrefText string, ids ...int) string {
	u, _ := url.Parse("https://bugzilla.redhat.com/buglist.cgi?f1=bug_id&list_id=11100046&o1=anyexact&query_format=advanced")
	e := u.Query()
	stringIds := make([]string, len(ids))
	for i := range stringIds {
		stringIds[i] = fmt.Sprintf("%d", ids[i])
	}
	e.Add("v1", strings.Join(stringIds, ","))
	u.RawQuery = e.Encode()
	return fmt.Sprintf("<%s|%s>", u.String(), hrefText)
}
