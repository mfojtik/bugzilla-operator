package blockers

import (
	"context"
	"fmt"
	"net/url"
	"strings"

	"github.com/eparis/bugzilla"
	"github.com/openshift/library-go/pkg/controller/factory"
	"github.com/openshift/library-go/pkg/operator/events"
	"k8s.io/apimachinery/pkg/util/sets"

	"github.com/mfojtik/bugzilla-operator/pkg/cache"
	"github.com/mfojtik/bugzilla-operator/pkg/operator/bugutil"
	"github.com/mfojtik/bugzilla-operator/pkg/operator/config"
	"github.com/mfojtik/bugzilla-operator/pkg/operator/controller"
	"github.com/mfojtik/bugzilla-operator/pkg/slack"
)

type BlockersReporter struct {
	controller.ControllerContext
	config     config.OperatorConfig
	components []string
}

const (
	blockerIntro = "Hi there!\nIt appears you have %d bugs assigned to you and these bugs are _%s_ *release blockers*:\n\n"
	blockerOutro = "\n\nPlease keep eyes on these today!"

	triageIntro = "Hi there!\nI found %d untriaged bugs assigned to you:\n\n"
	triageOutro = "\n\nPlease make sure all these have the _Severity_ field set and the _Target Release_ set, so I can stop bothering you :-)\n\n"
)

var (
	seriousKeywords = []string{
		"ServiceDeliveryBlocker",
		"TestBlocker",
		"UpgradeBlocker",
	}
)

func NewBlockersReporter(ctx controller.ControllerContext, components []string, schedule []string, operatorConfig config.OperatorConfig,
	recorder events.Recorder) factory.Controller {
	c := &BlockersReporter{
		ControllerContext: ctx,
		config:            operatorConfig,
		components:        components,
	}
	return factory.New().WithSync(c.sync).ResyncSchedule(schedule...).ToController("BlockersReporter", recorder)
}

type triageResult struct {
	seriousKeywordsIDs    map[string][]int
	blockers              []string
	blockerIDs            []int
	needUpcomingSprint    []string
	needUpcomingSprintIDs []int
	staleCount            int
	priorityCount         map[string]int
	severityCount         map[string]int
}

func triageBugs(currentTargetRelease string, bugs ...bugzilla.Bug) triageResult {
	r := triageResult{
		priorityCount: map[string]int{},
		severityCount: map[string]int{},
	}
	for _, bug := range bugs {
		keywords := sets.NewString(bug.Keywords...)
		for _, keyword := range seriousKeywords {
			if keywords.Has(keyword) {
				if r.seriousKeywordsIDs == nil {
					r.seriousKeywordsIDs = make(map[string][]int)
				}
				r.seriousKeywordsIDs[keyword] = append(r.seriousKeywordsIDs[keyword], bug.ID)
			}
		}

		if strings.Contains(bug.Whiteboard, "LifecycleStale") {
			r.staleCount++
		}

		r.severityCount[bug.Severity]++
		r.priorityCount[bug.Priority]++

		if !keywords.Has("UpcomingSprint") {
			r.needUpcomingSprint = append(r.needUpcomingSprint, bugutil.FormatBugMessage(bug))
			r.needUpcomingSprintIDs = append(r.needUpcomingSprintIDs, bug.ID)
		}

		targetRelease := "---"
		if len(bug.TargetRelease) > 0 {
			targetRelease = bug.TargetRelease[0]
		}

		if targetRelease == currentTargetRelease || targetRelease == "---" {
			r.blockers = append(r.blockers, bugutil.FormatBugMessage(bug))
			r.blockerIDs = append(r.blockerIDs, bug.ID)
		}
	}

	return r
}

type notificationMap struct {
	seriousKeywordsIDs map[string][]int

	blockers   map[string][]string
	blockerIDs map[string][]int

	needTriage    map[string][]string
	needTriageIDs map[string][]int

	priorityCount map[string]int
	severityCount map[string]int
	staleCount    int
}

func (c *BlockersReporter) sync(ctx context.Context, syncCtx factory.SyncContext) error {
	client := c.NewBugzillaClient(ctx)
	slackClient := c.SlackClient(ctx)

	channelReport, triagedBugs, err := Report(ctx, client, syncCtx.Recorder(), &c.config, c.components)
	if err != nil {
		return err
	}

	for person, notifications := range triagedBugs.blockers {
		if len(notifications) == 0 {
			continue
		}
		message := fmt.Sprintf("%s%s%s", fmt.Sprintf(blockerIntro, len(notifications), c.config.Release.CurrentTargetRelease), strings.Join(notifications, "\n"), fmt.Sprintf(blockerOutro))
		if err := slackClient.MessageEmail(person, message); err != nil {
			syncCtx.Recorder().Warningf("DeliveryFailed", "Failed to deliver:\n\n%s\n\n to %q: %v", message, person, err)
		}
	}

	if err := slackClient.MessageChannel(channelReport); err != nil {
		syncCtx.Recorder().Warningf("DeliveryFailed", "Failed to deliver stats to channel: %v", err)
	}

	// send debug stats
	c.sendStatsForPeople(*triagedBugs, slackClient)
	return nil
}

func getBugsQuery(config *config.OperatorConfig, components []string, targetRelease []string) bugzilla.Query {
	return bugzilla.Query{
		Classification: []string{"Red Hat"},
		Product:        []string{"OpenShift Container Platform"},
		Status:         []string{"NEW", "ASSIGNED", "POST", "ON_DEV"},
		Component:      components,
		TargetRelease:  targetRelease,
		Advanced: []bugzilla.AdvancedQuery{
			{
				Field: "bug_severity",
				Op:    "notequals",
				Value: "low",
			},
			{
				Field: "priority",
				Op:    "notequals",
				Value: "low",
			},
		},
		IncludeFields: []string{
			"id",
			"assigned_to",
			"keywords",
			"status",
			"resolution",
			"summary",
			"changeddate",
			"severity",
			"priority",
			"target_release",
			"whiteboard",
		},
	}
}

func Report(ctx context.Context, client cache.BugzillaClient, recorder events.Recorder, config *config.OperatorConfig, components []string) (string, *notificationMap, error) {
	activeBugsQuery := getBugsQuery(config, components, append([]string{"---"}, config.Release.TargetReleases...))
	triageBlockerQuery := getBugsQuery(config, components, append([]string{"---"}, config.Release.CurrentTargetRelease))

	activeBugs, err := client.Search(activeBugsQuery)
	if err != nil {
		recorder.Warningf("BugSearchFailed", err.Error())
		return "", nil, err
	}

	triageResult := &notificationMap{
		seriousKeywordsIDs: make(map[string][]int),
		blockers:           make(map[string][]string),
		blockerIDs:         make(map[string][]int),
		needTriage:         make(map[string][]string),
		needTriageIDs:      make(map[string][]int),
		priorityCount:      make(map[string]int),
		severityCount:      make(map[string]int),
	}

	peopleBugsMap := map[string][]bugzilla.Bug{}
	for _, b := range activeBugs {
		peopleBugsMap[b.AssignedTo] = append(peopleBugsMap[b.AssignedTo], *b)
	}

	for person, assignedBugs := range peopleBugsMap {
		result := triageBugs(config.Release.CurrentTargetRelease, assignedBugs...)

		if result.seriousKeywordsIDs != nil {
			for key, value := range result.seriousKeywordsIDs {
				triageResult.seriousKeywordsIDs[key] = append(triageResult.seriousKeywordsIDs[key], value...)
			}
		}

		triageResult.blockers[person] = result.blockers
		triageResult.blockerIDs[person] = result.blockerIDs

		for severity, count := range result.severityCount {
			triageResult.severityCount[severity] += count
		}
		for priority, count := range result.priorityCount {
			triageResult.priorityCount[priority] += count
		}
		triageResult.staleCount += result.staleCount
	}

	channelStats := getStatsForChannel(
		config.Release.CurrentTargetRelease,
		len(activeBugs),
		triageResult.seriousKeywordsIDs,
		triageResult.blockers,
		activeBugsQuery,
		triageBlockerQuery,
		triageResult.severityCount,
		triageResult.priorityCount,
		triageResult.staleCount,
	)

	report := fmt.Sprintf("\n:bug: *Today 4.x Bug Report:* :bug:\n%s\n", strings.Join(channelStats, "\n"))

	return report, triageResult, nil
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

func (c *BlockersReporter) sendStatsForPeople(triage notificationMap, slackClient slack.ChannelClient) {
	var messages []string
	for person, b := range triage.blockers {
		if len(b) > 0 {
			messages = append(messages, fmt.Sprintf("> %s: %d active bugs", makeBugzillaLink(person, triage.blockerIDs[person]...), len(b)))
		}
	}
	for person, b := range triage.needTriage {
		if len(b) > 0 {
			messages = append(messages, fmt.Sprintf("> %s: %d to needTriage", makeBugzillaLink(person, triage.needTriageIDs[person]...), len(b)))
		}
	}
	slackClient.MessageAdminChannel(strings.Join(messages, "\n"))
}

func getStatsForChannel(targetRelease string, activeBugsCount int, seriousKeywordsIDs map[string][]int, triageBlockers map[string][]string, activeBugsQuery, triageBlockerQuery bugzilla.Query, severity, priority map[string]int, stale int) []string {
	sortedPrioNames := []string{
		"urgent",
		"high",
		"medium",
		"low",
		"unspecified",
	}
	severityMessages := []string{}
	for _, p := range sortedPrioNames {
		if severity[p] > 0 {
			severityMessages = append(severityMessages, fmt.Sprintf("%d _%s_", severity[p], p))
		}
	}
	priorityMessages := []string{}
	for _, p := range sortedPrioNames {
		if priority[p] > 0 {
			priorityMessages = append(priorityMessages, fmt.Sprintf("%d _%s_", priority[p], p))
		}
	}
	triageBlockerCount := 0
	for p := range triageBlockers {
		triageBlockerCount += len(triageBlockers[p])
	}

	activeBugsQueryURL, _ := url.Parse("https://bugzilla.redhat.com/buglist.cgi?" + activeBugsQuery.Values().Encode())
	triageBlockerQueryURL, _ := url.Parse("https://bugzilla.redhat.com/buglist.cgi?" + triageBlockerQuery.Values().Encode())

	lines := []string{
		fmt.Sprintf("> All active 4.x and 3.11 Bugs: <%s|%d>", activeBugsQueryURL.String(), activeBugsCount),
		fmt.Sprintf("> Bugs Severity Breakdown: %s", strings.Join(severityMessages, ", ")),
		fmt.Sprintf("> Bugs Priority Breakdown: %s", strings.Join(priorityMessages, ", ")),
		fmt.Sprintf("> %s Release Blockers Count: <%s|%d>", targetRelease, triageBlockerQueryURL.String(), triageBlockerCount),
		fmt.Sprintf("> Bugs Marked as _LifecycleStale_: <https://bugzilla.redhat.com/buglist.cgi?cmdtype=dorem&remaction=run&namedcmd=openshift-group-b-lifecycle-stale&sharer_id=290313|%d>", stale),
	}

	if seriousKeywordsIDs != nil {
		for _, keyword := range seriousKeywords {
			if bugIDs := seriousKeywordsIDs[keyword]; len(bugIDs) > 0 {
				keywordURL := makeBugzillaLink(fmt.Sprintf("%d", len(bugIDs)), bugIDs...)
				lines = append(lines, fmt.Sprintf("> Bugs with _%s_: %s", keyword, keywordURL))
			}
		}
	}

	return lines
}
