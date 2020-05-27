package blockers

import (
	"context"
	"fmt"
	"strings"

	"github.com/eparis/bugzilla"
	"github.com/openshift/library-go/pkg/controller/factory"
	"github.com/openshift/library-go/pkg/operator/events"
	"k8s.io/apimachinery/pkg/util/sets"

	"github.com/mfojtik/bugzilla-operator/pkg/cache"
	"github.com/mfojtik/bugzilla-operator/pkg/operator/bugutil"
	"github.com/mfojtik/bugzilla-operator/pkg/operator/config"
	"github.com/mfojtik/bugzilla-operator/pkg/slack"
)

type BlockersReporter struct {
	config config.OperatorConfig

	newBugzillaClient             func() cache.BugzillaClient
	slackClient, slackDebugClient slack.ChannelClient
}

const (
	blockerIntro = "Hi there!\nIt appears you have %d bugs assigned to you and these bugs are _%s_ *release blockers*:\n\n"
	blockerOutro = "\n\nPlease keep eyes on these today!"

	triageIntro = "Hi there!\nI found %d untriaged bugs assigned to you:\n\n"
	triageOutro = "\n\nPlease make sure all these have the _Severity_ field set and the _Target Release_ set, so I can stop bothering you :-)\n\n"
)

func NewBlockersReporter(operatorConfig config.OperatorConfig, scheduleInformer factory.Informer, newBugzillaClient func() cache.BugzillaClient, slackClient, slackDebugClient slack.ChannelClient, recorder events.Recorder) factory.Controller {
	c := &BlockersReporter{
		config:            operatorConfig,
		newBugzillaClient: newBugzillaClient,
		slackClient:       slackClient,
		slackDebugClient:  slackDebugClient,
	}
	return factory.New().WithSync(c.sync).WithInformers(scheduleInformer).ToController("BlockersReporter", recorder)
}

type triageResult struct {
	blockers           []string
	needTriage         []string
	needUpcomingSprint []string
	staleCount         int
	priorityCount      map[string]int
	severityCount      map[string]int
}

func triageBug(currentTargetRelease string, bugs ...bugzilla.Bug) triageResult {
	r := triageResult{
		priorityCount: map[string]int{},
		severityCount: map[string]int{},
	}
	for _, bug := range bugs {
		if strings.Contains(bug.DevelWhiteboard, "LifecycleStale") {
			r.staleCount++
			continue
		}

		r.severityCount[bug.Severity]++
		r.priorityCount[bug.Priority]++

		keywords := sets.NewString(bug.Keywords...)
		if !keywords.Has("UpcomingSprint") {
			r.needUpcomingSprint = append(r.needUpcomingSprint, bugutil.FormatBugMessage(bug))
		}

		targetRelease := "---"
		if len(bug.TargetRelease) > 0 {
			targetRelease = bug.TargetRelease[0]
		}

		if bug.Severity == "unspecified" || targetRelease == "---" {
			r.needTriage = append(r.needTriage, bugutil.FormatBugMessage(bug))
		}

		if targetRelease == currentTargetRelease || targetRelease == "---" {
			r.blockers = append(r.blockers, bugutil.FormatBugMessage(bug))
		}
	}

	return r
}

type notificationMap struct {
	blockers       map[string][]string
	triage         map[string][]string
	upcomingSprint map[string][]string
	priorityCount  map[string]int
	severityCount  map[string]int
	staleCount     int
}

func (c *BlockersReporter) sync(ctx context.Context, syncCtx factory.SyncContext) error {
	client := c.newBugzillaClient()

	channelReport, triageResult, err := Report(ctx, client, syncCtx.Recorder(), &c.config)
	if err != nil {
		return err
	}

	for person, notifications := range triageResult.blockers {
		if len(notifications) == 0 {
			continue
		}
		message := fmt.Sprintf("%s%s%s", fmt.Sprintf(blockerIntro, len(notifications), c.config.Release.CurrentTargetRelease), strings.Join(notifications, "\n"), fmt.Sprintf(blockerOutro))
		if err := c.slackClient.MessageEmail(person, message); err != nil {
			syncCtx.Recorder().Warningf("DeliveryFailed", "Failed to deliver:\n\n%s\n\n to %q: %v", message, person, err)
		}
	}

	for assignee, notifications := range triageResult.triage {
		if len(notifications) == 0 {
			continue
		}
		message := fmt.Sprintf("%s%s%s", fmt.Sprintf(triageIntro, len(notifications)), strings.Join(notifications, "\n"), fmt.Sprintf(triageOutro))
		if err := c.slackClient.MessageEmail(assignee, message); err != nil {
			syncCtx.Recorder().Warningf("DeliveryFailed", "Failed to deliver:\n\n%s\n\n to %q: %v", message, assignee, err)
		}
	}

	if err := c.slackClient.MessageChannel(channelReport); err != nil {
		syncCtx.Recorder().Warningf("DeliveryFailed", "Failed to deliver stats to channel: %v", err)
	}

	// send debug stats
	c.sendStatsForPeople(triageResult.blockers, triageResult.triage, triageResult.upcomingSprint)
	return nil
}

func getBlockerList(client cache.BugzillaClient, config *config.OperatorConfig) ([]*bugzilla.Bug, error) {
	return client.Search(bugzilla.Query{
		Classification: []string{"Red Hat"},
		Product:        []string{"OpenShift Container Platform"},
		Status:         []string{"NEW", "ASSIGNED", "POST", "ON_DEV"},
		Component:      config.Components,
		TargetRelease:  config.Release.TargetReleases,
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
			"cf_devel_whiteboard",
		},
	})
}

func Report(ctx context.Context, client cache.BugzillaClient, recorder events.Recorder, config *config.OperatorConfig) (string, *notificationMap, error) {
	blockerBugs, err := getBlockerList(client, config)
	if err != nil {
		recorder.Warningf("BugSearchFailed", err.Error())
		return "", nil, err
	}

	triageResult := &notificationMap{
		blockers:       make(map[string][]string),
		triage:         make(map[string][]string),
		upcomingSprint: make(map[string][]string),
		priorityCount:  make(map[string]int),
		severityCount:  make(map[string]int),
	}

	peopleBugsMap := map[string][]bugzilla.Bug{}
	for _, b := range blockerBugs {
		peopleBugsMap[b.AssignedTo] = append(peopleBugsMap[b.AssignedTo], *b)
	}

	for person, assignedBugs := range peopleBugsMap {
		result := triageBug(config.Release.CurrentTargetRelease, assignedBugs...)

		triageResult.blockers[person] = result.blockers
		triageResult.triage[person] = result.needTriage
		triageResult.upcomingSprint[person] = result.needUpcomingSprint

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
		len(blockerBugs),
		triageResult.blockers,
		triageResult.triage,
		triageResult.upcomingSprint,
		triageResult.severityCount,
		triageResult.priorityCount,
		triageResult.staleCount,
	)
	report := fmt.Sprintf("\n:bug: *Today 4.x Bug Report:* :bug:\n%s\n", strings.Join(channelStats, "\n"))

	return report, triageResult, nil
}

func (c *BlockersReporter) sendStatsForPeople(blockers, triage, upcomingSprint map[string][]string) {
	var messages []string
	for person, b := range blockers {
		if len(b) > 0 {
			messages = append(messages, fmt.Sprintf("> %s: %d blockers", person, len(b)))
		}
	}
	for person, b := range triage {
		if len(b) > 0 {
			messages = append(messages, fmt.Sprintf("> %s: %d to triage", person, len(b)))
		}
	}
	for person, b := range upcomingSprint {
		if len(b) > 0 {
			messages = append(messages, fmt.Sprintf("> %s: %d to apply _UpcomingSprint_", person, len(b)))
		}
	}
	c.slackDebugClient.MessageChannel(strings.Join(messages, "\n"))
}

func getStatsForChannel(targetRelease string, totalCount int, blockers, triage, upcomingSprint map[string][]string, severity, priority map[string]int, stale int) []string {
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
	totalTriageCount := 0
	for p := range triage {
		totalTriageCount += len(triage[p])
	}
	totalTargetBlockerCount := 0
	for p := range blockers {
		totalTargetBlockerCount += len(blockers[p])
	}
	needUpcomingSprint := len(upcomingSprint)
	// we can have bug here :-)
	if needUpcomingSprint < 0 {
		needUpcomingSprint = 0
	}
	return []string{
		fmt.Sprintf("> All Active 4.x and 3.11 Bugs: <https://bugzilla.redhat.com/buglist.cgi?cmdtype=dorem&remaction=run&namedcmd=openshift-group-b-active-blockers&sharer_id=290313|%d>", totalCount-stale),
		fmt.Sprintf("> Bugs Severity Breakdown: %s", strings.Join(severityMessages, ", ")),
		fmt.Sprintf("> Bugs Priority Breakdown: %s", strings.Join(priorityMessages, ", ")),
		fmt.Sprintf("> %s Release Blockers Count: <https://bugzilla.redhat.com/buglist.cgi?cmdtype=dorem&remaction=run&namedcmd=openshift-group-b-current-blockers&sharer_id=290313|%d>", targetRelease,
			totalTargetBlockerCount),
		fmt.Sprintf("> Active Bugs Without _UpcomingSprint_: <https://bugzilla.redhat.com/buglist.cgi?cmdtype=dorem&remaction=run&namedcmd=openshift-group-b-blockers-upcoming&sharer_id=290313|%d>", needUpcomingSprint),
		fmt.Sprintf("> Untriaged Bugs: <https://bugzilla.redhat.com/buglist.cgi?cmdtype=dorem&remaction=run&namedcmd=openshift-group-b-triage&sharer_id=290313|%d>", totalTriageCount),
		fmt.Sprintf("> Bugs Marked as _LifecycleStale_: <https://bugzilla.redhat.com/buglist.cgi?cmdtype=dorem&remaction=run&namedcmd=openshift-group-b-lifecycle-staleCount&sharer_id=290313|%d>", stale),
	}
}
