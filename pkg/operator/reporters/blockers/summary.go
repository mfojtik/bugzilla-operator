package blockers

import (
	"fmt"
	"net/url"
	"strings"

	"github.com/eparis/bugzilla"
	"k8s.io/apimachinery/pkg/util/sets"
)

var (
	seriousKeywords = []string{
		"ServiceDeliveryBlocker",
		"TestBlocker",
		"UpgradeBlocker",
	}
)

type bugSummary struct {
	serious             map[string][]int
	blockerPlus         []int
	blockerQuestionmark []int
	toTriage            []int
	needUpcomingSprint  []int
	urgent              []int
	staleCount          int
	priorityCount       map[string]int
	severityCount       map[string]int
	withCustomerCase    int

	currentReleaseCount             int
	currentReleaseCustomerCaseCount int
	currentReleaseCICount           int

	noTargetReleaseCount int

	ciBugsCount int
}

func summarizeBugs(currentTargetRelease string, bugs ...*bugzilla.Bug) bugSummary {
	r := bugSummary{
		priorityCount: map[string]int{},
		severityCount: map[string]int{},
		serious:       map[string][]int{},
	}
	for _, bug := range bugs {
		keywords := sets.NewString(bug.Keywords...)
		for _, keyword := range seriousKeywords {
			if keywords.Has(keyword) {
				r.serious[keyword] = append(r.serious[keyword], bug.ID)
			}
		}

		if hasActiveCustomerCase(bug) {
			r.withCustomerCase++
		}

		if strings.Contains(bug.Whiteboard, "tag-ci") {
			r.ciBugsCount++
		}

		if strings.Contains(bug.Whiteboard, "LifecycleStale") {
			r.staleCount++
		}

		r.severityCount[bug.Severity]++
		r.priorityCount[bug.Priority]++

		if bug.Priority == "urgent" || (bug.Severity == "urgent" && bug.Priority == "unspecified") {
			r.urgent = append(r.urgent, bug.ID)
		}

		if !keywords.Has("UpcomingSprint") {
			r.needUpcomingSprint = append(r.needUpcomingSprint, bug.ID)
		}

		targetRelease := "---"

		if len(bug.TargetRelease) > 0 {
			targetRelease = bug.TargetRelease[0]
		}
		if targetRelease == "---" {
			r.noTargetReleaseCount++
		}

		if hasFlag(bug, "blocker", "+") && targetRelease == currentTargetRelease {
			r.blockerPlus = append(r.blockerPlus, bug.ID)
			r.serious["blocker+"] = append(r.serious["blocker+"], bug.ID)
		}

		if hasFlag(bug, "blocker", "?") && (targetRelease == currentTargetRelease || targetRelease == "---") {
			r.blockerQuestionmark = append(r.blockerQuestionmark, bug.ID)
			r.serious["blocker?"] = append(r.serious["blocker?"], bug.ID)
		}

		// Triage means people will be notified about bugs that require attention.
		// In this case:
		// - All bugs in NEW state require to be ASSIGNED
		// - All bugs in ASSIGNED (or other) state require to have priority AND severity set
		triageState := sets.NewString("NEW", "")
		if triageState.Has(bug.Status) {
			r.toTriage = append(r.toTriage, bug.ID)
		} else if needTriage(bug) {
			r.toTriage = append(r.toTriage, bug.ID)
		}

		if targetRelease == currentTargetRelease {
			if strings.Contains(bug.Whiteboard, "tag-ci") {
				r.currentReleaseCICount++
			}
			if hasActiveCustomerCase(bug) {
				r.currentReleaseCustomerCaseCount++
			}
			r.currentReleaseCount++
		}
	}

	return r
}

func needTriage(bug *bugzilla.Bug) bool {
	return bug.Priority == "unspecified" || bug.Priority == "" || bug.Severity == "unspecified" || bug.Severity == ""
}

func hasActiveCustomerCase(b *bugzilla.Bug) bool {
	for _, eb := range b.ExternalBugs {
		if eb.Type.Type == "SFDC" && eb.ExternalStatus != "Closed" {
			return true
		}
	}
	return false
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
