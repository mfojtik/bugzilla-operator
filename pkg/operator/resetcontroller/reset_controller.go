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

	var bugsToReset []*bugzilla.Bug
	var errors []error

	gotInfoBugs, err := getGotInfoBugsToReset(client, c.config)
	if err != nil {
		syncCtx.Recorder().Warningf("GotInfoSearchFailed", err.Error())
		errors = append(errors, err)
	}
	bugsToReset = append(bugsToReset, gotInfoBugs...)

	gotKeywordBugs, err := getKeywordsBugsToReset(client, c.config)
	if err != nil {
		syncCtx.Recorder().Warningf("GotKeywordSearchFailed", err.Error())
	}

	bugsToReset = append(bugsToReset, gotKeywordBugs...)

	var resetBugLinks []string
	for _, bug := range bugsToReset {
		if err := client.UpdateBug(bug.ID, bugzilla.BugUpdate{
			DevWhiteboard: "LifecycleReset",
			Comment: &bugzilla.BugComment{
				Body: "The LifecycleStale keyword was removed, because the needinfo? flag was reset or the bug received blocker/security keyword.\nThe bug assignee was notified.",
			},
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
		c.slackDebugClient.MessageChannel(fmt.Sprintf("%s reset: %s", bugutil.BugCountPlural(len(resetBugLinks), true), strings.Join(resetBugLinks, ", ")))
	}

	return errorutil.NewAggregate(errors)
}

func getKeywordsBugsToReset(client cache.BugzillaClient, c config.OperatorConfig) ([]*bugzilla.Bug, error) {
	return client.Search(bugzilla.Query{
		Classification: []string{"Red Hat"},
		Product:        []string{"OpenShift Container Platform"},
		Status:         []string{"NEW", "ASSIGNED", "POST", "ON_DEV"},
		Component:      c.Components,
		Advanced: []bugzilla.AdvancedQuery{
			{
				Field: "keywords",
				Op:    "substring",
				Value: "Blocker",
			},
			{
				Field: "keywords",
				Op:    "substring",
				Value: "Security",
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
			"keywords",
			"reporter",
			"severity",
			"priority",
		},
	})
}

func getGotInfoBugsToReset(client cache.BugzillaClient, c config.OperatorConfig) ([]*bugzilla.Bug, error) {
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
			{
				Field: "cf_devel_whiteboard",
				Op:    "notsubstring",
				Value: "LifecycleRotten",
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
