package poststalecontroller

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/mfojtik/bugzilla-operator/pkg/operator/bugutil"

	"github.com/eparis/bugzilla"
	"github.com/mfojtik/bugzilla-operator/pkg/cache"
	"github.com/mfojtik/bugzilla-operator/pkg/operator/config"
	"github.com/mfojtik/bugzilla-operator/pkg/operator/controller"
	"github.com/openshift/library-go/pkg/controller/factory"
	"github.com/openshift/library-go/pkg/operator/events"
)

// PostStaleBugController monitor bugs that are in POST state and report them.
type PostStaleBugController struct {
	controller.ControllerContext
	config config.OperatorConfig
}

func NewPostStaleBugController(ctx controller.ControllerContext, operatorConfig config.OperatorConfig, recorder events.Recorder) factory.Controller {
	c := &PostStaleBugController{ctx, operatorConfig}
	return factory.New().WithSync(c.sync).ResyncEvery(12*time.Hour).ToController("PostStaleBugController", recorder)
}

func (c *PostStaleBugController) sync(ctx context.Context, syncCtx factory.SyncContext) (err error) {
	client := c.NewBugzillaClient(ctx)
	slackClient := c.SlackClient(ctx)

	bugs, err := getNewBugs(client, c.config)
	if err != nil {
		syncCtx.Recorder().Warningf("BuglistFailed", err.Error())
		return err
	}
	bugList := []string{}
	for _, b := range bugs {
		bugList = append(bugList, bugutil.FormatBugMessage(*b))
	}

	return slackClient.MessageAdminChannel(
		fmt.Sprintf("Found %d *POST* bugs with high or urgent severity waiting for longer than 3 days:\n%s", len(bugList), strings.Join(bugList, "\n")),
	)
}

func getNewBugs(client cache.BugzillaClient, c config.OperatorConfig) ([]*bugzilla.Bug, error) {
	aq := bugzilla.AdvancedQuery{
		Field: "days_elapsed",
		Op:    "greaterthan",
		Value: "3",
	}

	return client.Search(bugzilla.Query{
		Classification: []string{"Red Hat"},
		Product:        []string{"OpenShift Container Platform"},
		Status:         []string{"POST"},
		Severity:       []string{"urgent", "high"},
		Component:      c.Components.List(),
		Advanced:       []bugzilla.AdvancedQuery{aq},
		IncludeFields: []string{
			"id",
			"assigned_to",
			"component",
			"severity",
			"priority",
			"summary",
			"status",
		},
	})
}
