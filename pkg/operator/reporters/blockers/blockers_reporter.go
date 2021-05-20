package blockers

import (
	"context"
	"fmt"
	"net/url"
	"strings"

	"github.com/eparis/bugzilla"
	"github.com/mfojtik/bugzilla-operator/pkg/cache"
	"github.com/mfojtik/bugzilla-operator/pkg/operator/bugutil"
	"github.com/mfojtik/bugzilla-operator/pkg/operator/config"
	"github.com/mfojtik/bugzilla-operator/pkg/operator/controller"
	"github.com/mfojtik/bugzilla-operator/pkg/slack"
	"github.com/openshift/library-go/pkg/controller/factory"
	"github.com/openshift/library-go/pkg/operator/events"
)

type BlockersReporter struct {
	controller.ControllerContext
	config                                            config.OperatorConfig
	components                                        []string
	notifyChannel                                     bool
	toTriageReminder, blockerReminder, urgentReminder bool
}

const (
	urgentIntro = "You have *%d urgent bugs*%s:\n\n"
	urgentOutro = "\n\nWe are expected to actively work on these before anything else!"

	blockerIntro = "You have *%d blocker+ bugs*%s:\n\n"
	blockerOutro = "\n\nPlease keep eyes on these, they will risk the upcoming release if not finished in time!"

	triageIntro = "You have *%d untriaged bugs*%s:\n\n"
	triageOutro = "\n\nPlease make sure all these have the _Severity_, _Priority_ and _Target Release_ set, and move to ASSIGNED, so I can stop bothering you :-)\n\n"
)

func NewChannelBlockersReporter(ctx controller.ControllerContext, components []string, schedule []string, operatorConfig config.OperatorConfig, recorder events.Recorder) factory.Controller {
	c := &BlockersReporter{
		ControllerContext: ctx,
		config:            operatorConfig,
		components:        components,
		notifyChannel:     true,
	}
	return factory.New().WithSync(c.sync).ResyncSchedule(schedule...).ToController("BlockersReporter", recorder)
}

func NewToTriageReminder(ctx controller.ControllerContext, components []string, schedule []string, operatorConfig config.OperatorConfig, recorder events.Recorder) factory.Controller {
	c := &BlockersReporter{
		ControllerContext: ctx,
		config:            operatorConfig,
		components:        components,
		toTriageReminder:  true,
	}
	return factory.New().WithSync(c.sync).ResyncSchedule(schedule...).ToController("ToTriageReminder", recorder)
}

func NewBlockerReminder(ctx controller.ControllerContext, components []string, schedule []string, operatorConfig config.OperatorConfig, recorder events.Recorder) factory.Controller {
	c := &BlockersReporter{
		ControllerContext: ctx,
		config:            operatorConfig,
		components:        components,
		blockerReminder:   true,
	}
	return factory.New().WithSync(c.sync).ResyncSchedule(schedule...).ToController("BlockerReminder", recorder)
}

func NewUrgentReminder(ctx controller.ControllerContext, components []string, schedule []string, operatorConfig config.OperatorConfig, recorder events.Recorder) factory.Controller {
	c := &BlockersReporter{
		ControllerContext: ctx,
		config:            operatorConfig,
		components:        components,
		urgentReminder:    true,
	}
	return factory.New().WithSync(c.sync).ResyncSchedule(schedule...).ToController("UrgentReminder", recorder)
}

func (c *BlockersReporter) sync(ctx context.Context, syncCtx factory.SyncContext) error {
	client := c.NewBugzillaClient(ctx)
	slackClient := c.SlackClient(ctx)

	channelReport, summary, bugs, err := Report(ctx, client, syncCtx.Recorder(), &c.config, c.components)
	if err != nil {
		return err
	}

	byID := map[int]bugzilla.Bug{}
	for _, b := range bugs {
		byID[b.ID] = *b
	}

	perPerson := func(ids []int) (map[string][]int, map[string][]string) {
		perPersonLines := map[string][]string{}
		perPersonIDs := map[string][]int{}
		for _, id := range ids {
			b, ok := byID[id]
			if !ok {
				continue
			}
			perPersonLines[b.AssignedTo] = append(perPersonLines[b.AssignedTo], bugutil.FormatBugMessage(b))
			perPersonIDs[b.AssignedTo] = append(perPersonIDs[b.AssignedTo], id)
		}
		return perPersonIDs, perPersonLines
	}

	perPersonToTriageIDs, perPersonToTriage := perPerson(summary.toTriage)
	perPersonBlockerPlusIDs, perPersonBlockerPlus := perPerson(summary.blockerPlus)
	perPersonUrgentIDs, perPersonUrgent := perPerson(summary.urgent)

	notifyPersons := func(intro, suffix string, perPersonBugs map[string][]string, outro string) {
		for person, lines := range perPersonBugs {
			if len(lines) == 0 {
				continue
			}
			message := fmt.Sprintf("%s%s%s", fmt.Sprintf(intro, len(lines), suffix), strings.Join(lines, "\n"), outro)
			if err := slackClient.MessageEmail(person, message); err != nil {
				syncCtx.Recorder().Warningf("DeliveryFailed", "Failed to deliver:\n\n%s\n\n to %q: %v", message, person, err)
			}
		}
	}

	if c.toTriageReminder {
		notifyPersons(triageIntro, "", perPersonToTriage, triageOutro)
	}
	if c.blockerReminder {
		notifyPersons(blockerIntro, fmt.Sprintf("for the %s release", c.config.Release.CurrentTargetRelease), perPersonBlockerPlus, blockerOutro)
	}
	if c.urgentReminder {
		notifyPersons(urgentIntro, "", perPersonUrgent, urgentOutro)
	}

	if c.notifyChannel {
		if err := slackClient.MessageChannel(channelReport); err != nil {
			syncCtx.Recorder().Warningf("DeliveryFailed", "Failed to deliver stats to channel: %v", err)
		}

		c.sendAdminDebugStats(slackClient, perPersonBlockerPlusIDs, perPersonToTriageIDs, perPersonUrgentIDs)
	}
	return nil
}

func getBugsQuery(config *config.OperatorConfig, components []string, targetRelease []string) bugzilla.Query {
	return bugzilla.Query{
		Classification: []string{"Red Hat"},
		Product:        []string{"OpenShift Container Platform"},
		Status:         []string{"NEW", "ASSIGNED", "POST", "ON_DEV"},
		Component:      components,
		TargetRelease:  targetRelease,
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
			"flags",
		},
	}
}

func Report(ctx context.Context, client cache.BugzillaClient, recorder events.Recorder, config *config.OperatorConfig, components []string) (string, *bugSummary, []*bugzilla.Bug, error) {
	allReleasesQuery := getBugsQuery(config, components, append([]string{"---"}, config.Release.TargetReleases...))
	currentReleaseQeury := getBugsQuery(config, components, append([]string{"---"}, config.Release.CurrentTargetRelease))

	bugs, err := client.Search(allReleasesQuery)
	if err != nil {
		recorder.Warningf("BugSearchFailed", err.Error())
		return "", nil, nil, err
	}

	summary := summarizeBugs(config.Release.CurrentTargetRelease, bugs...)
	channelStats := getStatsForChannel(
		config.Release.CurrentTargetRelease,
		len(bugs),
		summary,
		allReleasesQuery,
		currentReleaseQeury,
	)

	report := fmt.Sprintf("\n:bug: *Today 4.x Bug Report:* :bug:\n%s\n", strings.Join(channelStats, "\n"))
	return report, &summary, bugs, nil
}

func (c *BlockersReporter) sendAdminDebugStats(slackClient slack.ChannelClient, perPersonBlockers, perPersonToTriage, perPersonUrgent map[string][]int) {
	var messages []string
	for person, bs := range perPersonBlockers {
		if len(bs) > 0 {
			messages = append(messages, fmt.Sprintf("> %s: %d blocker+ bugs", makeBugzillaLink(person, perPersonBlockers[person]...), len(bs)))
		}
	}
	for person, bs := range perPersonToTriage {
		if len(bs) > 0 {
			messages = append(messages, fmt.Sprintf("> %s: %d bugs that need triage", makeBugzillaLink(person, perPersonToTriage[person]...), len(bs)))
		}
	}
	for person, bs := range perPersonUrgent {
		if len(bs) > 0 {
			messages = append(messages, fmt.Sprintf("> %s: %d urgent bugs", makeBugzillaLink(person, perPersonUrgent[person]...), len(bs)))
		}
	}
	slackClient.MessageAdminChannel(strings.Join(messages, "\n"))
}

func getStatsForChannel(targetRelease string, activeBugsCount int, summary bugSummary, allReleasesQuery, currentReleaseQuery bugzilla.Query) []string {
	sortedPrioNames := []string{
		"urgent",
		"high",
		"medium",
		"low",
		"unspecified",
	}
	severityMessages := []string{}
	for _, p := range sortedPrioNames {
		if summary.severityCount[p] > 0 {
			severityMessages = append(severityMessages, fmt.Sprintf("%d _%s_", summary.severityCount[p], p))
		}
	}
	priorityMessages := []string{}
	for _, p := range sortedPrioNames {
		if summary.priorityCount[p] > 0 {
			priorityMessages = append(priorityMessages, fmt.Sprintf("%d _%s_", summary.priorityCount[p], p))
		}
	}

	ciBugsQuery := allReleasesQuery
	ciBugsQuery.Advanced = []bugzilla.AdvancedQuery{
		{
			Field: "status_whiteboard",
			Op:    "substring",
			Value: "tag-ci",
		},
	}

	allReleasesQueryURL, _ := url.Parse("https://bugzilla.redhat.com/buglist.cgi?" + allReleasesQuery.Values().Encode())
	currentReleaseQueryURL, _ := url.Parse("https://bugzilla.redhat.com/buglist.cgi?" + currentReleaseQuery.Values().Encode())
	ciBugsQueryURL, _ := url.Parse("https://bugzilla.redhat.com/buglist.cgi?" + ciBugsQuery.Values().Encode())

	lines := []string{
		fmt.Sprintf("> All Releases Bugs: <%s|%d> _(<%s|%d> CI, %d customer case)_", allReleasesQueryURL.String(), activeBugsCount, ciBugsQueryURL.String(), summary.ciBugsCount, summary.withCustomerCase),
		fmt.Sprintf("> All Current (*%s*) Release Bugs: <%s|%d> _(%d CI, %d customer case)_", targetRelease, currentReleaseQueryURL.String(), summary.currentReleaseCount, summary.currentReleaseCICount, summary.currentReleaseCustomerCaseCount),
		fmt.Sprintf("> Bugs without target release: %d", summary.noTargetReleaseCount),
		fmt.Sprintf("> "),
		fmt.Sprintf("> Bugs Severity Breakdown: %s", strings.Join(severityMessages, ", ")),
		fmt.Sprintf("> Bugs Priority Breakdown: %s", strings.Join(priorityMessages, ", ")),
		fmt.Sprintf("> Bugs with no activity for more than 30d: <https://bugzilla.redhat.com/buglist.cgi?cmdtype=dorem&remaction=run&namedcmd=openshift-group-b-lifecycle-stale&sharer_id=290313|%d>", summary.staleCount),
	}

	for keyword, ids := range summary.serious {
		if len(ids) > 0 {
			keywordURL := makeBugzillaLink(fmt.Sprintf("%d", len(ids)), ids...)
			lines = append(lines, fmt.Sprintf("> Bugs with _%s_: %s", keyword, keywordURL))
		}
	}

	return lines
}
