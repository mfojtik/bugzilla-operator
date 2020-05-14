package stalecontroller

import (
	"context"
	"strings"
	"time"

	"github.com/davecgh/go-spew/spew"
	"github.com/openshift/library-go/pkg/controller/factory"
	"github.com/openshift/library-go/pkg/operator/events"
	errutil "k8s.io/apimachinery/pkg/util/errors"
	"k8s.io/klog"

	"github.com/eparis/bugzilla"

	"github.com/mfojtik/bugzilla-operator/pkg/operator/config"
)

const bugzillaEndpoint = "https://bugzilla.redhat.com"

type StaleController struct {
	config config.OperatorConfig
}

func NewStaleController(operatorConfig config.OperatorConfig, recorder events.Recorder) factory.Controller {
	c := &StaleController{
		config: operatorConfig,
	}
	return factory.New().WithSync(c.sync).ResyncEvery(1*time.Hour).ToController("StaleController", recorder)
}

func (c *StaleController) newClient() bugzilla.Client {
	return bugzilla.NewClient(func() []byte {
		return []byte(c.config.Credentials.DecodedAPIKey())
	}, bugzillaEndpoint).WithCGIClient(c.config.Credentials.DecodedUsername(), c.config.Credentials.DecodedPassword())
}

func (c *StaleController) handleBug(client bugzilla.Client, bug bugzilla.Bug) (*bugzilla.BugUpdate, error) {
	bugInfo, err := client.GetBug(bug.ID)
	if err != nil {
		return nil, err
	}
	klog.Infof("#%d (S:%s, R:%s, A:%s): %s", bug.ID, bugInfo.Severity, bugInfo.Creator, bugInfo.AssignedTo, trunc(bug.Summary))
	bugUpdate := bugzilla.BugUpdate{
		DevWhiteboard: c.config.Lists.Stale.Action.AddKeyword,
	}
	needInfoPerson := []string{}
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
		bugUpdate.Priority = degrade(transitions, bug.Priority)
	}
	if transitions := c.config.Lists.Stale.Action.SeverityTransitions; len(transitions) > 0 {
		bugUpdate.Severity = degrade(transitions, bug.Severity)
	}
	if len(c.config.Lists.Stale.Action.AddComment) > 0 {
		bugUpdate.Comment = &bugzilla.BugComment{
			Body: c.config.Lists.Stale.Action.AddComment,
		}
	}
	return &bugUpdate, nil

}

func (c *StaleController) sync(ctx context.Context, syncCtx factory.SyncContext) error {
	client := c.newClient()
	staleBugs, err := client.BugList(c.config.Lists.Stale.Name, c.config.Lists.Stale.SharerID)
	if err != nil {
		syncCtx.Recorder().Warningf("BuglistFailed", err.Error())
		return err
	}

	var errors []error
	klog.Infof("Received %d stale bugs", len(staleBugs))
	for _, bug := range staleBugs {
		bugUpdate, err := c.handleBug(client, bug)
		if err != nil {
			errors = append(errors, err)
			continue
		}
		if err := client.UpdateBug(bug.ID, *bugUpdate); err != nil {
			errors = append(errors, err)
		}
		klog.Infof("#%d updated: %s", bug.ID, spew.Sdump(bugUpdate))
	}

	return errutil.NewAggregate(errors)
}

func trunc(in string) string {
	if len(in) >= 120 {
		return in[0:120] + "..."
	}
	return in
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
