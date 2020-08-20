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
	"k8s.io/klog"

	"github.com/mfojtik/bugzilla-operator/pkg/cache"
	"github.com/mfojtik/bugzilla-operator/pkg/operator/bugutil"
	"github.com/mfojtik/bugzilla-operator/pkg/operator/config"
	"github.com/mfojtik/bugzilla-operator/pkg/operator/controller"
	"github.com/mfojtik/bugzilla-operator/pkg/operator/stalecontroller"
)

type ResetStaleController struct {
	controller.ControllerContext
	config config.OperatorConfig
}

func NewResetStaleController(ctx controller.ControllerContext, operatorConfig config.OperatorConfig, recorder events.Recorder) factory.Controller {
	c := &ResetStaleController{
		ControllerContext: ctx,
		config:            operatorConfig,
	}
	return factory.New().WithSync(c.sync).ResyncEvery(1*time.Hour).ToController("ResetStaleController", recorder)
}

func (c *ResetStaleController) sync(ctx context.Context, syncCtx factory.SyncContext) error {
	client := c.NewBugzillaClient(ctx)
	slackClient := c.SlackClient(ctx)

	var errors []error

	reasons := map[int][]string{}
	bugsToReset := map[int]*bugzilla.Bug{}

	gotInfoBugs, err := getBugsWithNoNeedInfoToReset(client, c.config)
	if err != nil {
		syncCtx.Recorder().Warningf("GotInfoSearchFailed", err.Error())
		errors = append(errors, err)
	}
	for _, bug := range gotInfoBugs {
		bugsToReset[bug.ID] = bug
		reasons[bug.ID] = append(reasons[bug.ID], "the needinfo? flag was reset")
	}

	gotKeywordBugs, err := getBugsWithKeywordsToReset(client, c.config)
	if err != nil {
		syncCtx.Recorder().Warningf("GotKeywordSearchFailed", err.Error())
	}
	for _, bug := range gotKeywordBugs {
		bugsToReset[bug.ID] = bug
		reasons[bug.ID] = append(reasons[bug.ID], "the bug received blocker/security keyword")
	}

	gotStatusBugs, err := getInvalidStatusBugsToReset(client, c.config)
	if err != nil {
		syncCtx.Recorder().Warningf("GotStatusSearchFailed", err.Error())
	}
	for _, bug := range gotStatusBugs {
		bugsToReset[bug.ID] = bug
		reasons[bug.ID] = append(reasons[bug.ID], "the bug moved to QE")
	}

	gotRecentlyCommentedBugs, err := getRecentlyCommentedBugsToReset(client, c.config)
	if err != nil {
		syncCtx.Recorder().Warningf("GotRecentlyCommentedSearchFailed", err.Error())
	}
	for _, bug := range gotRecentlyCommentedBugs {
		bugsToReset[bug.ID] = bug
		reasons[bug.ID] = append(reasons[bug.ID], "the bug got commented on recently")
	}

	var resetBugLinks []string
	for id, bug := range bugsToReset {
		if err := client.UpdateBug(bug.ID, bugzilla.BugUpdate{
			DevWhiteboard: "LifecycleReset",
			Comment: &bugzilla.BugComment{
				Body: fmt.Sprintf("The LifecycleStale keyword was removed because %s.\nThe bug assignee was notified.", strings.Join(reasons[id], " and ")),
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
		message := fmt.Sprintf("Following bug _LifecycleStale_ was *removed* after %s:\n%s\n", strings.Join(reasons[id], " and "), bugutil.FormatBugMessage(*bug))

		if err := slackClient.MessageEmail(bug.AssignedTo, message); err != nil {
			syncCtx.Recorder().Warningf("DeliveryFailed", "Failed to deliver close message to %q: %v", bug.AssignedTo, err)
		}
		if err := slackClient.MessageEmail(bug.Creator, message); err != nil {
			syncCtx.Recorder().Warningf("DeliveryFailed", "Failed to deliver close message to %q: %v", bug.Creator, err)

		}
	}

	// Notify admin
	if len(resetBugLinks) > 0 {
		slackClient.MessageAdminChannel(fmt.Sprintf("%s reset: %s", bugutil.BugCountPlural(len(resetBugLinks), true), strings.Join(resetBugLinks, ", ")))
	}

	return errorutil.NewAggregate(errors)
}

func getBugsWithKeywordsToReset(client cache.BugzillaClient, c config.OperatorConfig) ([]*bugzilla.Bug, error) {
	return client.Search(bugzilla.Query{
		Classification: []string{"Red Hat"},
		Product:        []string{"OpenShift Container Platform"},
		Status:         []string{"NEW", "ASSIGNED", "POST", "ON_DEV"},
		Component:      c.Components.List(),
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
			"keywords",
			"reporter",
			"severity",
			"priority",
		},
	})
}

func getInvalidStatusBugsToReset(client cache.BugzillaClient, c config.OperatorConfig) ([]*bugzilla.Bug, error) {
	return client.Search(bugzilla.Query{
		Classification: []string{"Red Hat"},
		Product:        []string{"OpenShift Container Platform"},
		Status:         []string{"MODIFIED", "VERIFIED", "ON_QA"},
		Component:      c.Components.List(),
		Advanced: []bugzilla.AdvancedQuery{
			{
				Field: "status_whiteboard",
				Op:    "substring",
				Value: "LifecycleStale",
			},
			{
				Field: "status_whiteboard",
				Op:    "notsubstring",
				Value: "LifecycleRotten",
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
}

func getBugsWithNoNeedInfoToReset(client cache.BugzillaClient, c config.OperatorConfig) ([]*bugzilla.Bug, error) {
	return client.Search(bugzilla.Query{
		Classification: []string{"Red Hat"},
		Product:        []string{"OpenShift Container Platform"},
		Status:         []string{"NEW", "ASSIGNED", "POST", "ON_DEV"},
		Component:      c.Components.List(),
		Advanced: []bugzilla.AdvancedQuery{
			{
				Field: "flagtypes.name",
				Op:    "notsubstring",
				Value: "needinfo",
			},
			{
				Field: "status_whiteboard",
				Op:    "substring",
				Value: "LifecycleStale",
			},
			{
				Field: "status_whiteboard",
				Op:    "notsubstring",
				Value: "LifecycleRotten",
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
}

func getRecentlyCommentedBugsToReset(client cache.BugzillaClient, c config.OperatorConfig) ([]*bugzilla.Bug, error) {
	staleBugs, err := client.Search(bugzilla.Query{
		Classification: []string{"Red Hat"},
		Product:        []string{"OpenShift Container Platform"},
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

	var toBeReset []*bugzilla.Bug
	for _, bug := range staleBugs {
		lastSignificantChangeAt, err := stalecontroller.LastSignificantChangeAt(client, bug)
		if err != nil {
			klog.Error(err)
			continue
		}

		if lastSignificantChangeAt.After(time.Now().Add(-stalecontroller.MinimumStaleDuration)) {
			toBeReset = append(toBeReset, bug)
		}
	}

	return toBeReset, nil
}
