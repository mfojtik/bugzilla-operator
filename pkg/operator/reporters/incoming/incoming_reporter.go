package incoming

import (
	"context"
	"fmt"
	"strings"

	"github.com/mfojtik/bugzilla-operator/pkg/operator/bugutil"

	"github.com/eparis/bugzilla"
	"github.com/openshift/library-go/pkg/controller/factory"
	"github.com/openshift/library-go/pkg/operator/events"

	"github.com/mfojtik/bugzilla-operator/pkg/cache"
	"github.com/mfojtik/bugzilla-operator/pkg/operator/config"
	"github.com/mfojtik/bugzilla-operator/pkg/operator/controller"
)

// IncomingReport reports bugs that are NEW and haven't been assigned yet.
// To track new bugs, we chose to tag bugs we have seen with 'AssigneeNotified' keyword (in DevWhiteboard).
// This reported will notify assignees about new bugs based on the reporter schedule (2x a day).
// Additionally, a report of new bugs will be sent to the status channel.
type IncomingReporter struct {
	controller.ControllerContext
	config config.OperatorConfig
}

func (c *IncomingReporter) sync(ctx context.Context, syncContext factory.SyncContext) error {
	client := c.NewBugzillaClient(ctx)
	slackClient := c.SlackClient(ctx)

	channelReport, assigneeReports, err := Report(ctx, client, syncContext.Recorder(), &c.config)
	if err != nil {
		return err
	}
	if len(assigneeReports) == 0 {
		return nil
	}

	// In 95% cases this will hit the default component assignees.
	for assignee, bugs := range assigneeReports {
		message := fmt.Sprintf("%s\n\n> Please set severity/priority on the bug(s) above and assign to a team member.\n", strings.Join(bugs.reports, "\n"))
		if err := slackClient.MessageEmail(assignee, message); err != nil {
			syncContext.Recorder().Warningf("DeliveryFailed", "Failed to deliver:\n\n%s\n\n to %q: %v", message, assignee, err)
			continue
		}
		for _, id := range bugs.bugIDs {
			if err := c.markAsReported(client, id); err != nil {
				syncContext.Recorder().Warningf("MarkNotifiedFailed", "Failed to mark bug #%d with AssigneeNotified: %v", id, err)
			}
		}
	}

	if err := slackClient.MessageChannel(channelReport); err != nil {
		syncContext.Recorder().Warningf("DeliveryFailed", "Failed to deliver new bugs: %v", err)
		return err
	}

	return nil
}

func NewIncomingReporter(ctx controller.ControllerContext, schedule []string, operatorConfig config.OperatorConfig, recorder events.Recorder) factory.Controller {
	c := &IncomingReporter{
		ControllerContext: ctx,
		config:            operatorConfig,
	}
	return factory.New().WithSync(c.sync).ResyncSchedule(schedule...).ToController("IncomingReporter", recorder)
}

type AssigneeReport struct {
	bugIDs  []int
	reports []string
}

func Report(ctx context.Context, client cache.BugzillaClient, recorder events.Recorder, config *config.OperatorConfig) (string, map[string]AssigneeReport, error) {
	incomingBugs, err := getIncomingBugsList(client, config)
	if err != nil {
		recorder.Warningf("BugSearchFailed", err.Error())
		return "", nil, err
	}

	var channelReport []string
	assigneeReports := map[string]AssigneeReport{}

	for _, bug := range incomingBugs {
		bugMessage := fmt.Sprintf(":bugzilla: %s", bugutil.FormatBugMessage(*bug))
		channelReport = append(channelReport, "> "+bugMessage)
		currentReport, ok := assigneeReports[bug.AssignedTo]
		if ok {
			currentReport.reports = append(currentReport.reports, bugMessage)
			currentReport.bugIDs = append(currentReport.bugIDs, bug.ID)
			continue
		}
		assigneeReports[bug.AssignedTo] = AssigneeReport{
			bugIDs:  []int{bug.ID},
			reports: []string{bugMessage},
		}
	}

	return strings.Join(channelReport, "\n"), assigneeReports, nil
}

func (c *IncomingReporter) markAsReported(client cache.BugzillaClient, id int) error {
	return client.UpdateBug(id, bugzilla.BugUpdate{DevWhiteboard: "AssigneeNotified"})
}

func getIncomingBugsList(client cache.BugzillaClient, config *config.OperatorConfig) ([]*bugzilla.Bug, error) {
	return client.Search(bugzilla.Query{
		Classification: []string{"Red Hat"},
		Product:        []string{"OpenShift Container Platform"},
		Status:         []string{"NEW", "ASSIGNED"},
		Component:      config.Components.List(),
		Advanced: []bugzilla.AdvancedQuery{
			{
				Field: "cf_devel_whiteboard",
				Op:    "notsubstring",
				Value: "AssigneeNotified",
			},
		},
		IncludeFields: []string{
			"id",
			"assigned_to",
			"keywords",
			"status",
			"component",
			"resolution",
			"summary",
			"severity",
			"priority",
			"target_release",
			"whiteboard",
		},
	})
}
