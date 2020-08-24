package closecontroller

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/eparis/bugzilla"
	"github.com/openshift/library-go/pkg/controller/factory"
	"github.com/openshift/library-go/pkg/operator/events"
	errorutil "k8s.io/apimachinery/pkg/util/errors"
	"k8s.io/klog"

	"github.com/mfojtik/bugzilla-operator/pkg/cache"
	"github.com/mfojtik/bugzilla-operator/pkg/operator/bugutil"
	"github.com/mfojtik/bugzilla-operator/pkg/operator/config"
	"github.com/mfojtik/bugzilla-operator/pkg/operator/controller"
	"github.com/mfojtik/bugzilla-operator/pkg/operator/stalecontroller"
)

var priorityTransitions = []config.Transition{
	{From: "medium", To: "low"},
	{From: "unspecified", To: "low"},
}

type CloseStaleController struct {
	controller.ControllerContext
	config config.OperatorConfig
}

func NewCloseStaleController(ctx controller.ControllerContext, operatorConfig config.OperatorConfig, recorder events.Recorder) factory.Controller {
	c := &CloseStaleController{ctx, operatorConfig}
	return factory.New().WithSync(c.sync).ResyncEvery(1*time.Hour).ToController("CloseStaleController", recorder)
}

func (c *CloseStaleController) sync(ctx context.Context, syncCtx factory.SyncContext) error {
	client := c.NewBugzillaClient(ctx)
	staleBugs, err := getBugsToClose(client, c.config)
	if err != nil {
		syncCtx.Recorder().Warningf("BuglistFailed", err.Error())
		return err
	}
	slackClient := c.SlackClient(ctx)

	var errors []error
	var closedBugLinks []string
	for _, bug := range staleBugs {
		if err := client.UpdateBug(bug.ID, bugzilla.BugUpdate{
			Status:     "CLOSED",
			Resolution: "WONTFIX",
			Comment: &bugzilla.BugComment{
				Body: c.config.StaleBugCloseComment,
			},
			Priority: bugutil.DegradePriority(priorityTransitions, bug.Priority),
		}); err != nil {
			syncCtx.Recorder().Warningf("BugCloseFailed", "Failed to close bug #%d: %v", bug.ID, err)
			errors = append(errors, err)
			continue
		}

		// in some cases, the search query return zero assignee or creator, which cause the slack messages failed to deliver.
		// in that case, try to get the bug directly, which should populate all fields.
		if len(bug.AssignedTo) == 0 || len(bug.Creator) == 0 {
			b, err := client.GetBug(bug.ID)
			if err == nil {
				bug = b
			}
		}

		closedBugLinks = append(closedBugLinks, bugutil.GetBugURL(*bug))
		message := fmt.Sprintf("Following bug was automatically *closed* after being marked as _LifecycleStale_ for 7 days without update:\n%s\n", bugutil.FormatBugMessage(*bug))

		if err := slackClient.MessageEmail(bug.AssignedTo, message); err != nil {
			syncCtx.Recorder().Warningf("DeliveryFailed", "Failed to deliver close message to assignee %q: %v", bug.AssignedTo, err)
		}
		if err := slackClient.MessageEmail(bug.Creator, message); err != nil {
			syncCtx.Recorder().Warningf("DeliveryFailed", "Failed to deliver close message to reporter %q: %v", bug.Creator, err)

		}
	}

	// Notify admin
	if len(closedBugLinks) > 0 {
		slackClient.MessageAdminChannel(fmt.Sprintf("%s closed: %s", bugutil.BugCountPlural(len(closedBugLinks), true), strings.Join(closedBugLinks, ", ")))
	}

	return errorutil.NewAggregate(errors)
}

func getBugsToClose(client cache.BugzillaClient, c config.OperatorConfig) ([]*bugzilla.Bug, error) {
	staleBugs, err := client.Search(bugzilla.Query{
		Classification: []string{"Red Hat"},
		Product:        []string{"OpenShift Container Platform"},
		Status:         []string{"NEW", "ASSIGNED", "POST", "ON_DEV"},
		Component:      c.Components.List(),
		Advanced: []bugzilla.AdvancedQuery{
			{
				Field: "status_whiteboard",
				Op:    "substring",
				Value: "LifecycleStale",
			},
		},
		IncludeFields: []string{
			"id",
			"creation_time",
			"last_change_time",
			"assigned_to",
			"reporter",
			"severity",
			"priority",
		},
	})
	if err != nil {
		return nil, err
	}

	var toBeClosed []*bugzilla.Bug
	for _, bug := range staleBugs {
		lastSignificantChangeAt, err := stalecontroller.LastSignificantChangeAt(client, bug, c)
		if err != nil {
			klog.Error(err)
			continue
		}

		if lastSignificantChangeAt.Before(time.Now().Add(-stalecontroller.MinimumStaleDuration - 7*24*time.Hour)) {
			toBeClosed = append(toBeClosed, bug)
		}
	}

	return toBeClosed, nil
}
