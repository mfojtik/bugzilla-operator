package newcontroller

import (
	"context"
	"fmt"
	"strconv"
	"strings"
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

const stateKey = "new-bug-controller.state"

func NewNewBugController(ctx controller.ControllerContext, operatorConfig config.OperatorConfig, recorder events.Recorder) factory.Controller {
	c := &NewBugController{ctx, operatorConfig}
	return factory.New().WithSync(c.sync).ResyncEvery(1*time.Hour).ToController("NewBugController", recorder)
}

func (c *NewBugController) sync(ctx context.Context, syncCtx factory.SyncContext) (err error) {
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
		if persistErr := c.SetPersistentValue(ctx, stateKey, strconv.Itoa(lastID)); persistErr != nil {
			if err == nil {
				err = persistErr
			}
		}
	}()

	newBugs, err := getNewBugs(client, c.config, lastID)
	if err != nil {
		syncCtx.Recorder().Warningf("BuglistFailed", err.Error())
		return err
	}

	var errs []error
	ids := []string{}
	for i, b := range newBugs {
		if b.ID > lastID {
			lastID = b.ID
		}
		ids = append(ids, fmt.Sprintf("<https://bugzilla.redhat.com/show_bug.cgi?id=%d|#%d>", b.ID, b.ID))
		if i > 50 {
			ids = append(ids, fmt.Sprintf(" ... and %d more", len(newBugs)-50))
			break
		}
	}
	slackClient.MessageAdminChannel(fmt.Sprintf("Found new bugs: %s", strings.Join(ids, ", ")))

	// TODO: add interactivity and send to assignee

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
			Op:    "greaterthaneq",
			Value: "-24h", // last day
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
