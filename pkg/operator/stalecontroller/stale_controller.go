package stalecontroller

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

	"github.com/mfojtik/bugzilla-operator/pkg/operator/bugutil"
	"github.com/mfojtik/bugzilla-operator/pkg/operator/config"
	"github.com/mfojtik/bugzilla-operator/pkg/slack"
)

const bugzillaEndpoint = "https://bugzilla.redhat.com"

type StaleController struct {
	config      config.OperatorConfig
	slackClient slack.Client
}

func NewStaleController(operatorConfig config.OperatorConfig, slackClient slack.Client, recorder events.Recorder) factory.Controller {
	c := &StaleController{
		config:      operatorConfig,
		slackClient: slackClient,
	}
	return factory.New().WithSync(c.sync).ResyncEvery(1*time.Hour).ToController("StaleController", recorder)
}

func (c *StaleController) newClient() bugzilla.Client {
	return bugzilla.NewClient(func() []byte {
		return []byte(c.config.Credentials.DecodedAPIKey())
	}, bugzillaEndpoint).WithCGIClient(c.config.Credentials.DecodedUsername(), c.config.Credentials.DecodedPassword())
}

func (c *StaleController) handleBug(client bugzilla.Client, bug bugzilla.Bug) (*bugzilla.BugUpdate, *bugzilla.Bug, error) {
	bugInfo, err := client.GetBug(bug.ID)
	if err != nil {
		return nil, nil, err
	}
	klog.Infof("#%d (S:%s, P:%s, R:%s, A:%s): %s", bug.ID, bugInfo.Severity, bugInfo.Priority, bugInfo.Creator, bugInfo.AssignedTo, bug.Summary)
	bugUpdate := bugzilla.BugUpdate{
		DevWhiteboard: c.config.Lists.Stale.Action.AddKeyword,
	}
	var needInfoPerson []string
	if c.config.Lists.Stale.Action.NeedInfoFromCreator {
		needInfoPerson = append(needInfoPerson, bugInfo.Creator)
	}
	if c.config.Lists.Stale.Action.NeedInfoFromAssignee {
		needInfoPerson = append(needInfoPerson, bugInfo.AssignedTo)
	}
	if len(needInfoPerson) > 0 {
		flags := []bugzilla.FlagChange{}
		flags = append(flags, bugzilla.FlagChange{
			Name:      "needinfo",
			Status:    "?",
			Requestee: strings.Join(needInfoPerson, ","),
		})
		bugUpdate.Flags = flags
	}
	if transitions := c.config.Lists.Stale.Action.PriorityTransitions; len(transitions) > 0 {
		bugUpdate.Priority = degrade(transitions, bugInfo.Priority)
	}
	if transitions := c.config.Lists.Stale.Action.SeverityTransitions; len(transitions) > 0 {
		bugUpdate.Severity = degrade(transitions, bugInfo.Severity)
	}
	if len(c.config.Lists.Stale.Action.AddComment) > 0 {
		bugUpdate.Comment = &bugzilla.BugComment{
			Body: c.config.Lists.Stale.Action.AddComment,
		}
	}
	return &bugUpdate, bugInfo, nil
}

func (c *StaleController) sync(ctx context.Context, syncCtx factory.SyncContext) error {
	client := c.newClient()
	staleBugs, err := client.BugList(c.config.Lists.Stale.Name, c.config.Lists.Stale.SharerID)
	if err != nil {
		syncCtx.Recorder().Warningf("BuglistFailed", err.Error())
		return err
	}

	var errors []error

	klog.Infof("%d stale bugs found", len(staleBugs))
	notifications := map[string][]string{}

	staleBugLinks := []string{}
	for _, bug := range staleBugs {
		bugUpdate, bugInfo, err := c.handleBug(client, bug)
		if err != nil {
			errors = append(errors, err)
			continue
		}
		if err := client.UpdateBug(bug.ID, *bugUpdate); err != nil {
			errors = append(errors, err)
		}
		staleBugLinks = append(staleBugLinks, bugutil.FormatBugMessage(*bugInfo))
		notifications[bugInfo.AssignedTo] = append(notifications[bugInfo.AssignedTo], bugutil.FormatBugMessage(*bugInfo))
		notifications[bugInfo.Creator] = append(notifications[bugInfo.Creator], bugutil.FormatBugMessage(*bugInfo))
	}

	for target, messages := range notifications {
		message := fmt.Sprintf("Hi there!\nThese bugs you are assigned to were just marked as _LifecycleStale_:\n\n%s\n\nPlease review these and remove this flag if you think they are still valid bugs.",
			strings.Join(messages, "\n"))

		if err := c.slackClient.MessageEmail(target, message); err != nil {
			syncCtx.Recorder().Warningf("MessageFailed", fmt.Sprintf("Message to %q failed to send: %v", target, err))
		}
	}

	if len(notifications) > 0 {
		syncCtx.Recorder().Event("StaleBugs", fmt.Sprintf("Following notifications sent:\n%s\n", strings.Join(staleBugLinks, "\n")))
	}

	return errutil.NewAggregate(errors)
}

// degrade transition Priority and Severity fields one level down
func degrade(trans []config.Transition, in string) string {
	for _, t := range trans {
		if t.From == in {
			return t.To
		}
	}
	return ""
}
