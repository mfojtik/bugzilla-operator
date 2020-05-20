package blockers

import (
	"context"
	"fmt"
	"strings"
	"sync"

	"github.com/openshift/library-go/pkg/controller/factory"
	"github.com/openshift/library-go/pkg/operator/events"
	"k8s.io/apimachinery/pkg/util/sets"

	"github.com/eparis/bugzilla"

	"github.com/mfojtik/bugzilla-operator/pkg/operator/bugutil"
	"github.com/mfojtik/bugzilla-operator/pkg/operator/config"
	"github.com/mfojtik/bugzilla-operator/pkg/slack"
)

type BlockersReporter struct {
	config config.OperatorConfig

	newBugzillaClient             func() bugzilla.Client
	slackClient, slackDebugClient slack.ChannelClient
}

const (
	blockerIntro = "Hi there!\nIt appears you have %d bugs assigned to you and these bugs are _%s_ *release blockers*:\n\n"
	blockerOutro = "\n\nPlease keep eyes on these today!"

	triageIntro = "Hi there!\nI found %d untriaged bugs assigned to you:\n\n"
	triageOutro = "\n\nPlease make sure all these have the _Severity_ field set and the _Target Release_ set, so I can stop bothering you :-)\n\n"
)

func NewBlockersReporter(operatorConfig config.OperatorConfig, scheduleInformer factory.Informer, newBugzillaClient func() bugzilla.Client, slackClient, slackDebugClient slack.ChannelClient, recorder events.Recorder) factory.Controller {
	c := &BlockersReporter{
		config:            operatorConfig,
		newBugzillaClient: newBugzillaClient,
		slackClient:       slackClient,
		slackDebugClient:  slackDebugClient,
	}
	return factory.New().WithSync(c.sync).WithInformers(scheduleInformer).ToController("BlockersReporter", recorder)
}

type triageResult struct {
	blockers       []string
	needTriage     []string
	upcomingSprint []string
}

func triageBug(client bugzilla.Client, currentTargetRelease string, bugIDs ...int) triageResult {
	r := triageResult{}
	for _, id := range bugIDs {
		bug, err := client.GetBug(id)
		if err != nil {
			continue
		}
		keywords := sets.NewString(bug.Keywords...)
		if keywords.Has("UpcomingSprint") {
			r.upcomingSprint = append(r.upcomingSprint, bugutil.FormatBugMessage(*bug))
		}
		if len(bug.TargetRelease) == 0 {
			r.needTriage = append(r.needTriage, bugutil.FormatBugMessage(*bug))
			continue
		}

		if bug.Severity == "unspecified" || bug.TargetRelease[0] == "---" {
			r.needTriage = append(r.needTriage, bugutil.FormatBugMessage(*bug))
		}

		if bug.TargetRelease[0] == currentTargetRelease {
			r.blockers = append(r.blockers, bugutil.FormatBugMessage(*bug))
		}
	}

	return r
}

type notificationMap struct {
	blockers       map[string][]string
	triage         map[string][]string
	upcomingSprint map[string][]string
	sync.Mutex
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

	for person, notifications := range triageResult.triage {
		if len(notifications) == 0 {
			continue
		}
		message := fmt.Sprintf("%s%s%s", fmt.Sprintf(triageIntro, len(notifications)), strings.Join(notifications, "\n"), fmt.Sprintf(triageOutro))
		if err := c.slackClient.MessageEmail(person, message); err != nil {
			syncCtx.Recorder().Warningf("DeliveryFailed", "Failed to deliver:\n\n%s\n\n to %q: %v", message, person, err)
		}
	}

	if err := c.slackClient.MessageChannel(channelReport); err != nil {
		syncCtx.Recorder().Warningf("DeliveryFailed", "Failed to deliver stats to channel: %v", err)
	}

	// send debug stats
	c.sendStatsForPeople(triageResult.blockers, triageResult.triage, triageResult.upcomingSprint)
	return nil
}

func Report(ctx context.Context, client bugzilla.Client, recorder events.Recorder, config *config.OperatorConfig) (string, *notificationMap, error) {
	blockerBugs, err := client.BugList(config.Lists.Blockers.Name, config.Lists.Blockers.SharerID)
	if err != nil {
		recorder.Warningf("BuglistFailed", err.Error())
		return "", nil, err
	}

	interestingStatus := sets.NewString("NEW", "ASSIGNED", "POST", "ON_DEV")
	peopleBugsMap := map[string][]int{}
	for _, b := range blockerBugs {
		if !interestingStatus.Has(b.Status) {
			continue
		}
		peopleBugsMap[b.AssignedTo] = append(peopleBugsMap[b.AssignedTo], b.ID)
	}

	triageResult := &notificationMap{
		blockers:       make(map[string][]string),
		triage:         make(map[string][]string),
		upcomingSprint: make(map[string][]string),
	}

	var wg sync.WaitGroup
	for person, bugIDs := range peopleBugsMap {
		wg.Add(1)
		go func(person string, ids []int) {
			defer wg.Done()
			result := triageBug(client, config.Release.CurrentTargetRelease, ids...)
			triageResult.Lock()
			defer triageResult.Unlock()
			triageResult.blockers[person] = result.blockers
			triageResult.triage[person] = result.needTriage
			triageResult.upcomingSprint[person] = result.upcomingSprint
		}(person, bugIDs)
	}
	wg.Wait()

	channelStats := getStatsForChannel(config.Release.CurrentTargetRelease, len(blockerBugs), triageResult.blockers, triageResult.triage, triageResult.upcomingSprint)
	report := fmt.Sprintf("*Current Blocker Stats:*\n%s\n", strings.Join(channelStats, "\n"))

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

func getStatsForChannel(targetRelease string, totalCount int, blockers, triage, upcomingSprint map[string][]string) []string {
	totalTriageCount := 0
	for p := range triage {
		totalTriageCount += len(triage[p])
	}
	totalTargetBlockerCount := 0
	for p := range blockers {
		totalTargetBlockerCount += len(blockers[p])
	}
	needUpcomingSprint := totalCount - len(upcomingSprint)
	// we can have bug here :-)
	if needUpcomingSprint < 0 {
		needUpcomingSprint = 0
	}
	return []string{
		fmt.Sprintf("> All Blocker Bugs: <https://bugzilla.redhat.com/buglist.cgi?cmdtype=dorem&remaction=run&namedcmd=openshift-group-b-blockers&sharer_id=290313|%d>", totalCount),
		fmt.Sprintf("> %s Blocker Count: *%d*", targetRelease, totalTargetBlockerCount),
		fmt.Sprintf("> Need _UpcomingSprint_ keyword: <https://bugzilla.redhat.com/buglist.cgi?cmdtype=dorem&remaction=run&namedcmd=openshift-group-b-blockers-upcoming&sharer_id=290313|%d>", needUpcomingSprint),
		fmt.Sprintf("> Blockers Need Triage: <https://bugzilla.redhat.com/buglist.cgi?cmdtype=dorem&remaction=run&namedcmd=openshift-group-b-triage&sharer_id=290313|%d>", totalTriageCount),
	}
}
