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

	"github.com/mfojtik/bugzilla-operator/pkg/cache"
	"github.com/mfojtik/bugzilla-operator/pkg/operator/bugutil"
	"github.com/mfojtik/bugzilla-operator/pkg/operator/config"
	"github.com/mfojtik/bugzilla-operator/pkg/slack"
)

type CloseStaleController struct {
	config config.OperatorConfig

	newBugzillaClient             func() cache.BugzillaClient
	slackClient, slackDebugClient slack.ChannelClient
}

func NewCloseStaleController(operatorConfig config.OperatorConfig, newBugzillaClient func() cache.BugzillaClient, slackClient, slackDebugClient slack.ChannelClient, recorder events.Recorder) factory.Controller {
	c := &CloseStaleController{
		config:            operatorConfig,
		newBugzillaClient: newBugzillaClient,
		slackClient:       slackClient,
		slackDebugClient:  slackDebugClient,
	}
	return factory.New().WithSync(c.sync).ResyncEvery(1*time.Hour).ToController("CloseStaleController", recorder)
}

func (c *CloseStaleController) sync(ctx context.Context, syncCtx factory.SyncContext) error {
	client := c.newBugzillaClient()
	staleBugs, err := client.BugList(c.config.Lists.StaleClose.Name, c.config.Lists.StaleClose.SharerID)
	if err != nil {
		syncCtx.Recorder().Warningf("BuglistFailed", err.Error())
		return err
	}

	var errors []error
	var closedBugLinks []string
	for _, bug := range staleBugs {
		bugInfo, err := client.GetCachedBug(bug.ID, bugutil.LastChangeTimeToRevision(bug.LastChangeTime))
		if err != nil {
			syncCtx.Recorder().Warningf("BugInfoFailed", "Failed to query bug #%d: %v", bug.ID, err)
			errors = append(errors, err)
			continue
		}
		if err := client.UpdateBug(bug.ID, bugzilla.BugUpdate{
			Status:     c.config.Lists.StaleClose.Action.SetState,
			Resolution: c.config.Lists.StaleClose.Action.SetResolution,
			Comment: &bugzilla.BugComment{
				Body: c.config.Lists.StaleClose.Action.AddComment,
			},
			Priority: bugutil.DegradePriority(c.config.Lists.StaleClose.Action.PriorityTransitions, bugInfo.Priority),
		}); err != nil {
			syncCtx.Recorder().Warningf("BugCloseFailed", "Failed to close bug #%d: %v", bug.ID, err)
			errors = append(errors, err)
			continue
		}

		closedBugLinks = append(closedBugLinks, bugutil.GetBugURL(*bugInfo))
		message := fmt.Sprintf("Following bug was automatically *closed* after being marked as _LifecycleStale_ for 7 days without update:\n%s\n", bugutil.FormatBugMessage(*bugInfo))

		if err := c.slackClient.MessageEmail(bugInfo.AssignedTo, message); err != nil {
			syncCtx.Recorder().Warningf("DeliveryFailed", "Failed to deliver close message to %q: %v", bugInfo.AssignedTo, err)
		}
		if err := c.slackClient.MessageEmail(bugInfo.Creator, message); err != nil {
			syncCtx.Recorder().Warningf("DeliveryFailed", "Failed to deliver close message to %q: %v", bugInfo.Creator, err)

		}
	}

	// Notify admin
	if len(closedBugLinks) > 0 {
		c.slackDebugClient.MessageChannel(fmt.Sprintf("%s closed: %s", bugutil.BugCountPlural(len(closedBugLinks), true), strings.Join(closedBugLinks, ",")))
	}

	return errorutil.NewAggregate(errors)
}
