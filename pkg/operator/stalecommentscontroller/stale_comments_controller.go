package stalecommentscontroller

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/openshift/library-go/pkg/controller/factory"
	"github.com/openshift/library-go/pkg/operator/events"
	errutil "k8s.io/apimachinery/pkg/util/errors"
	"k8s.io/klog"

	"github.com/eparis/bugzilla"

	"github.com/mfojtik/bugzilla-operator/pkg/cache"
	"github.com/mfojtik/bugzilla-operator/pkg/operator/bugutil"
	"github.com/mfojtik/bugzilla-operator/pkg/operator/config"
	"github.com/mfojtik/bugzilla-operator/pkg/operator/controller"
)

var priorityTransitions = []config.Transition{
	{From: "high", To: "medium"},
	{From: "medium", To: "low"},
	{From: "unspecified", To: "low"},
}

type StaleCommentsController struct {
	controller.ControllerContext
	config config.OperatorConfig
}

func NewStaleCommentsController(ctx controller.ControllerContext, operatorConfig config.OperatorConfig, recorder events.Recorder) factory.Controller {
	c := &StaleCommentsController{
		ControllerContext: ctx,
		config:            operatorConfig,
	}
	return factory.New().WithSync(c.sync).ResyncEvery(1*time.Hour).ToController("StaleCommentsController", recorder)
}

func (c *StaleCommentsController) handleBug(bug bugzilla.Bug) (*bugzilla.BugUpdate, error) {
	klog.Infof("#%d (S:%s, P:%s, R:%s, A:%s): %s", bug.ID, bug.Severity, bug.Priority, bug.Creator, bug.AssignedTo, bug.Summary)
	bugUpdate := bugzilla.BugUpdate{
		Whiteboard: "LifecycleStale",
	}
	flags := []bugzilla.FlagChange{}
	flags = append(flags, bugzilla.FlagChange{
		Name:      "needinfo",
		Status:    "?",
		Requestee: bug.Creator,
	})
	bugUpdate.Flags = flags
	bugUpdate.Priority = bugutil.DegradePriority(priorityTransitions, bug.Priority)
	bugUpdate.Comment = &bugzilla.BugComment{
		Body: c.config.StaleBugComment,
	}
	return &bugUpdate, nil
}

var botCommentKeywords = []string{
	"PM Score",
	"UpcomingSprint",
	"This bug will be evaluated during the next sprint and prioritized appropriately.",
	"I am working on other high priority items. I will get to this bug next sprint.",
	"This bug will be evaluated next sprint.",
}

func (c *StaleCommentsController) sync(ctx context.Context, syncCtx factory.SyncContext) error {
	client := c.NewBugzillaClient(ctx)
	slackClient := c.SlackClient(ctx)
	candidates, err := getPotentiallyStaleBugs(client, c.config)
	if err != nil {
		syncCtx.Recorder().Warningf("BuglistFailed", err.Error())
		return err
	}

	var staleBugs []*bugzilla.Bug
	for _, bug := range candidates {
		comments, err := client.GetCachedBugComments(bug.ID, bug.LastChangeTime)
		if err != nil {
			syncCtx.Recorder().Warningf("GetCachedBugComments", err.Error())
			continue
		}

		recentlyChanged := false
	NextComment:
		for _, cmt := range comments {
			shortText := strings.Split(cmt.Text, "\n")[0]

			for _, keyword := range botCommentKeywords {
				if strings.Contains(cmt.Text, keyword) {
					klog.V(4).Infof("Ignoring bot comment for #%d due to keyword %q: %s", bug.ID, keyword, shortText)
					continue NextComment
				}
			}

			createdAt, err := time.Parse(time.RFC3339, cmt.Time)
			if err != nil {
				klog.Warningf("Skipping comment #%d of bug #%d because of time %q parse error: %v", cmt.Count, bug.ID, cmt.Time, err)
				continue
			}
			if createdAt.After(time.Now().Add(-time.Hour * 24 * 30)) {
				klog.V(4).Infof("Ignoring bug #%d because of recent comment #%d from %s: %s", bug.ID, cmt.Count, cmt.Time, shortText)
				recentlyChanged = true
				break
			}
		}

		if !recentlyChanged {
			staleBugs = append(staleBugs, bug)
		}
	}

	var errors []error

	notifications := map[string][]string{}

	staleBugLinks := []string{}
	for _, bug := range staleBugs {
		bugUpdate, err := c.handleBug(*bug)
		if err != nil {
			errors = append(errors, err)
			continue
		}
		if err := client.UpdateBug(bug.ID, *bugUpdate); err != nil {
			errors = append(errors, err)
		}
		// in some cases, the search query return zero assignee or creator, which cause the slack messages failed to deliver.
		// in that case, try to get the bug directly, which should populate all fields.
		if len(bug.AssignedTo) == 0 || len(bug.Creator) == 0 {
			b, err := client.GetBug(bug.ID)
			if err == nil {
				bug = b
			}
		}
		staleBugLinks = append(staleBugLinks, bugutil.FormatBugMessage(*bug))
		notifications[bug.AssignedTo] = append(notifications[bug.AssignedTo], bugutil.FormatBugMessage(*bug))
		notifications[bug.Creator] = append(notifications[bug.Creator], bugutil.FormatBugMessage(*bug))
	}

	for target, messages := range notifications {
		message := fmt.Sprintf("Hi there!\nThese bugs you are assigned to were just marked as _LifecycleStale_:\n\n%s\n\nPlease review these and remove this flag if you think they are still valid bugs.",
			strings.Join(messages, "\n"))

		if err := slackClient.MessageEmail(target, message); err != nil {
			syncCtx.Recorder().Warningf("MessageFailed", fmt.Sprintf("Message to %q failed to send: %v", target, err))
		}
	}

	if len(notifications) > 0 {
		syncCtx.Recorder().Event("StaleCommentsBugs", fmt.Sprintf("Following notifications sent:\n%s\n", strings.Join(staleBugLinks, "\n")))
	}

	return errutil.NewAggregate(errors)
}

func getPotentiallyStaleBugs(client cache.BugzillaClient, c config.OperatorConfig) ([]*bugzilla.Bug, error) {
	return client.Search(bugzilla.Query{
		Classification: []string{"Red Hat"},
		Product:        []string{"OpenShift Container Platform"},
		Status:         []string{"NEW", "ASSIGNED", "POST", "ON_DEV"},
		Component:      c.Components.List(),
		Advanced: []bugzilla.AdvancedQuery{
			{
				Field: "external_bugzilla.description",
				Op:    "notsubstring",
				Value: "Customer Portal",
			},
			{
				Field: "external_bugzilla.description",
				Op:    "notsubstring",
				Value: "Github",
			},
			{
				Field: "bug_severity",
				Op:    "notequals",
				Value: "urgent",
			},
			{
				Field: "short_desc",
				Op:    "notsubstring",
				Value: "CVE",
			},
			{
				Field: "keywords",
				Op:    "notsubstring",
				Value: "Security",
			},
			{
				Field: "keywords",
				Op:    "notsubstring",
				Value: "Blocker",
			},
		},
		IncludeFields: []string{
			"id",
			"assigned_to",
			"reporter",
			"keywords",
			"summary",
			"severity",
			"priority",
			"target_release",
			"whiteboard",
		},
	})
}
