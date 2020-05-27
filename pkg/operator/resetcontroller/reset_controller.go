package resetcontroller

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

type ResetStaleController struct {
	config config.OperatorConfig

	newBugzillaClient             func() cache.BugzillaClient
	slackClient, slackDebugClient slack.ChannelClient
}

func NewResetStaleController(operatorConfig config.OperatorConfig, newBugzillaClient func() cache.BugzillaClient, slackClient, slackDebugClient slack.ChannelClient, recorder events.Recorder) factory.Controller {
	c := &ResetStaleController{
		config:            operatorConfig,
		newBugzillaClient: newBugzillaClient,
		slackClient:       slackClient,
		slackDebugClient:  slackDebugClient,
	}
	return factory.New().WithSync(c.sync).ResyncEvery(1*time.Hour).ToController("ResetStaleController", recorder)
}

func (c *ResetStaleController) sync(ctx context.Context, syncCtx factory.SyncContext) error {
	client := c.newBugzillaClient()
	bugsToReset, err := client.BugList(c.config.Lists.ResetStale.Name, c.config.Lists.ResetStale.SharerID)
	if err != nil {
		syncCtx.Recorder().Warningf("BuglistFailed", err.Error())
		return err
	}

	var errors []error
	var resetBugLinks []string
	for _, bug := range bugsToReset {
		bugInfo, err := client.GetCachedBug(bug.ID, bugutil.LastChangeTimeToRevision(bug.LastChangeTime))
		if err != nil {
			syncCtx.Recorder().Warningf("BugInfoFailed", "Failed to query bug #%d: %v", bug.ID, err)
			errors = append(errors, err)
			continue
		}
		if err := client.UpdateBug(bug.ID, bugzilla.BugUpdate{
			DevWhiteboard: c.config.Lists.ResetStale.Action.AddKeyword,
			Flags: []bugzilla.FlagChange{
				{
					Name:      "needinfo",
					Status:    "?",
					Requestee: bugInfo.AssignedTo,
				},
			},
		}); err != nil {
			syncCtx.Recorder().Warningf("BugCloseFailed", "Failed to close bug #%d: %v", bug.ID, err)
			errors = append(errors, err)
			continue
		}

		resetBugLinks = append(resetBugLinks, bugutil.GetBugURL(*bugInfo))
		message := fmt.Sprintf("Following bug _LifecycleStale_ was *removed* after the _need_info?_ flag was reset:\n%s\n", bugutil.FormatBugMessage(*bugInfo))

		if err := c.slackClient.MessageEmail(bugInfo.AssignedTo, message); err != nil {
			syncCtx.Recorder().Warningf("DeliveryFailed", "Failed to deliver close message to %q: %v", bugInfo.AssignedTo, err)
		}
		if err := c.slackClient.MessageEmail(bugInfo.Creator, message); err != nil {
			syncCtx.Recorder().Warningf("DeliveryFailed", "Failed to deliver close message to %q: %v", bugInfo.Creator, err)

		}
	}

	// Notify admin
	if len(resetBugLinks) > 0 {
		c.slackDebugClient.MessageChannel(fmt.Sprintf("%s reset: %s", bugutil.BugCountPlural(len(resetBugLinks), true), strings.Join(resetBugLinks, ",")))
	}

	return errorutil.NewAggregate(errors)
}
