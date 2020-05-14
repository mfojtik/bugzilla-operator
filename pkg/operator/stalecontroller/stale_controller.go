package stalecontroller

import (
	"context"
	"time"

	"github.com/openshift/library-go/pkg/controller/factory"
	"github.com/openshift/library-go/pkg/operator/events"
	errutil "k8s.io/apimachinery/pkg/util/errors"
	"k8s.io/klog"

	"github.com/eparis/bugzilla"

	"github.com/mfojtik/bugzilla-operator/pkg/operator/config"
)

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
	}, "https://bugzilla.redhat.com").WithCGIClient(c.config.Credentials.DecodedUsername(), c.config.Credentials.DecodedPassword())
}

func (c *StaleController) sync(ctx context.Context, syncCtx factory.SyncContext) error {
	client := c.newClient()
	staleBugs, err := client.BugList(c.config.Lists.StaleListName, c.config.Lists.SharerID)
	if err != nil {
		syncCtx.Recorder().Warningf("BuglistFailed", err.Error())
		return err
	}

	var errors []error
	klog.Infof("Received %d stale bugs", len(staleBugs))
	for _, bug := range staleBugs {
		bugInfo, err := client.GetBug(bug.ID)
		if err != nil {
			errors = append(errors, err)
			continue
		}
		klog.Infof("#%d (S:%s, R:%s, A:%s): %s", bug.ID, bugInfo.Severity, bugInfo.Creator, bugInfo.AssignedTo, trunc(bug.Summary))
		if err := client.UpdateBug(bug.ID, bugzilla.BugUpdate{
			DevWhiteboard: c.config.DevWhiteboardFlag,
			Flags: []bugzilla.FlagChange{
				{
					Name:      "needinfo",
					Status:    "?",
					Requestee: bugInfo.Creator,
				},
			},
			Priority: degrade(bugInfo.Priority),
			Severity: degrade(bugInfo.Severity),
			Comment: &bugzilla.BugComment{
				Body: c.config.StaleBugComment,
			},
		}); err != nil {
			errors = append(errors, err)
		}
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
// TODO: move this to config
func degrade(in string) string {
	switch in {
	case "unspecified":
		return ""
	case "high":
		return "medium"
	case "medium":
		return "low"
	case "low":
		return "low"
	case "urgent":
		return "urgent"
	default:
		return ""
	}
}
