package newcontroller

import (
	"context"
	"fmt"
	"strconv"
	"time"

	"github.com/eparis/bugzilla"
	"github.com/openshift/library-go/pkg/controller/factory"
	"github.com/openshift/library-go/pkg/operator/events"
	errorutil "k8s.io/apimachinery/pkg/util/errors"
	"k8s.io/klog"

	"github.com/mfojtik/bugzilla-operator/pkg/cache"
	"github.com/mfojtik/bugzilla-operator/pkg/operator/config"
	"github.com/mfojtik/bugzilla-operator/pkg/operator/controller"
)

type NewBugController struct {
	controller.ControllerContext
	config config.OperatorConfig
}

const stateKey = "new-bug-controller/state"

func NewNewBugController(ctx controller.ControllerContext, operatorConfig config.OperatorConfig, recorder events.Recorder) factory.Controller {
	c := &NewBugController{ctx, operatorConfig}
	return factory.New().WithSync(c.sync).ResyncEvery(1*time.Hour).ToController("NewBugController", recorder)
}

func (c *NewBugController) sync(ctx context.Context, syncCtx factory.SyncContext) error {
	client := c.NewBugzillaClient(ctx)
	slackClient := c.SlackClient(ctx)

	lastID := 0
	if s, err := c.GetPersistentValue(ctx, stateKey); err != nil {
		return err
	} else if s != "" {
		lastID, err = strconv.Atoi(s)
		if err != nil {
			klog.Warningf("Cannot parse state value for %s: %v", stateKey, err)
			lastID = 0 // keep going
		}
	}
	defer func() {
		if err := c.SetPersistentValue(ctx, stateKey, strconv.Itoa(lastID)); err != nil {
			klog.Error(err)
		}
	}()

	newBugs, err := getNewBugs(client, c.config, lastID)
	if err != nil {
		syncCtx.Recorder().Warningf("BuglistFailed", err.Error())
		return err
	}

	var errs []error
	for _, b := range newBugs {
		if b.ID > lastID {
			lastID = b.ID
		}
		slackClient.MessageAdminChannel(fmt.Sprintf("Found new bug: <https://bugzilla.redhat.com/show_bug.cgi?id=%v|#%v %q>", b.ID, b.ID, b.Summary))

		// TODO: add interactivity and send to assignee
	}

	return errorutil.NewAggregate(errs)
}

func getNewBugs(client cache.BugzillaClient, c config.OperatorConfig, lastID int) ([]*bugzilla.Bug, error) {
	aq := bugzilla.AdvancedQuery{
		Field: "bug_id",
		Op:    "greaterthan",
		Value: strconv.Itoa(lastID),
	}
	if lastID == 0 {
		aq = bugzilla.AdvancedQuery{
			Field: "creation_ts",
			Op:    "greaterthan",
			Value: "-10m", // last 10 minute
		}
	}

	return client.Search(bugzilla.Query{
		Classification: []string{"Red Hat"},
		Product:        []string{"OpenShift Container Platform"},
		Status:         []string{"NEW"},
		Component:      c.Components.List(),
		Advanced:       []bugzilla.AdvancedQuery{aq},
		IncludeFields: []string{
			"id",
			"assigned_to",
			"component",
			"summary",
		},
	})
}
