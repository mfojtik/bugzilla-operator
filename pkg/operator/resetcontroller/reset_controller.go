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
	bugsToReset, err := getBugsToReset(client, c.config)
	if err != nil {
		syncCtx.Recorder().Warningf("BuglistFailed", err.Error())
		return err
	}

	var errors []error
	var resetBugLinks []string
	for _, bug := range bugsToReset {
		if err := client.UpdateBug(bug.ID, bugzilla.BugUpdate{
			DevWhiteboard: "LifecycleReset",
			Flags: []bugzilla.FlagChange{
				{
					Name:      "needinfo",
					Status:    "?",
					Requestee: bug.AssignedTo,
				},
			},
		}); err != nil {
			syncCtx.Recorder().Warningf("BugCloseFailed", "Failed to close bug #%d: %v", bug.ID, err)
			errors = append(errors, err)
			continue
		}

		resetBugLinks = append(resetBugLinks, bugutil.GetBugURL(*bug))
		message := fmt.Sprintf("Following bug _LifecycleStale_ was *removed* after the _need_info?_ flag was reset:\n%s\n", bugutil.FormatBugMessage(*bug))

		if err := c.slackClient.MessageEmail(bug.AssignedTo, message); err != nil {
			syncCtx.Recorder().Warningf("DeliveryFailed", "Failed to deliver close message to %q: %v", bug.AssignedTo, err)
		}
		if err := c.slackClient.MessageEmail(bug.Creator, message); err != nil {
			syncCtx.Recorder().Warningf("DeliveryFailed", "Failed to deliver close message to %q: %v", bug.Creator, err)

		}
	}

	// Notify admin
	if len(resetBugLinks) > 0 {
		c.slackDebugClient.MessageChannel(fmt.Sprintf("%s reset: %s", bugutil.BugCountPlural(len(resetBugLinks), true), strings.Join(resetBugLinks, ",")))
	}

	return errorutil.NewAggregate(errors)
}

func getBugsToReset(client cache.BugzillaClient, c config.OperatorConfig) ([]*bugzilla.Bug, error) {
	return client.Search(bugzilla.Query{
		Classification: []string{"Red Hat"},
		Product:        []string{"OpenShift Container Platform"},
		Status:         []string{"NEW", "ASSIGNED", "POST", "ON_DEV"},
		Component:      c.Components,
		Advanced: []bugzilla.AdvancedQuery{
			{
				Field: "flagtypes.name",
				Op:    "notsubstring",
				Value: "needinfo",
			},
			{
				Field: "cf_devel_whiteboard",
				Op:    "substring",
				Value: "LifecycleStale",
			},
		},
		IncludeFields: []string{
			"id",
			"assigned_to",
			"reporter",
			"severity",
			"priority",
		},
	})
}
