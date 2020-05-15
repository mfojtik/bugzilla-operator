package closed

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/eparis/bugzilla"
	"github.com/openshift/library-go/pkg/controller/factory"
	"github.com/openshift/library-go/pkg/operator/events"

	"github.com/mfojtik/bugzilla-operator/pkg/operator/config"
	"github.com/mfojtik/bugzilla-operator/pkg/slack"
)

const bugzillaEndpoint = "https://bugzilla.redhat.com"

type BlockersReporter struct {
	config      config.OperatorConfig
	slackClient slack.Client
}

func NewClosedReporter(operatorConfig config.OperatorConfig, slackClient slack.Client, recorder events.Recorder) factory.Controller {
	c := &BlockersReporter{
		config:      operatorConfig,
		slackClient: slackClient,
	}
	return factory.New().WithSync(c.sync).ResyncEvery(24*time.Hour).ToController("BlockersReporter", recorder)
}

func (c *BlockersReporter) newClient() bugzilla.Client {
	return bugzilla.NewClient(func() []byte {
		return []byte(c.config.Credentials.DecodedAPIKey())
	}, bugzillaEndpoint).WithCGIClient(c.config.Credentials.DecodedUsername(), c.config.Credentials.DecodedPassword())
}

func (c *BlockersReporter) sync(ctx context.Context, syncCtx factory.SyncContext) error {
	client := c.newClient()
	closedBugs, err := client.BugList(c.config.Lists.Closed.Name, c.config.Lists.Closed.SharerID)
	if err != nil {
		syncCtx.Recorder().Warningf("BuglistFailed", err.Error())
		return err
	}

	resolutionMap := map[string][]bugzilla.Bug{}
	for _, bug := range closedBugs {
		resolutionMap[bug.Resolution] = append(resolutionMap[bug.Resolution], bug)
	}

	message := []string{}
	for resolution, bugs := range resolutionMap {
		ids := []string{}
		for _, b := range bugs {
			ids = append(ids, fmt.Sprintf("<https://bugzilla.redhat.com/show_bug.cgi?id=%d|#%d>", b.ID, b.ID))
		}
		p := "bugs"
		if len(bugs) == 1 {
			p = "bug"
		}
		message = append(message, fmt.Sprintf("> %d %s closed as _%s_ (%s)", len(bugs), p, resolution, strings.Join(ids, ",")))
	}

	report := fmt.Sprintf("Bugs Closed in the last 24h:\n%s\n", strings.Join(message, "\n"))
	if err := c.slackClient.MessageChannel(report); err != nil {
		syncCtx.Recorder().Warningf("DeliveryFailed", "Failed to deliver closed bug counts: %v", err)
		return err
	}

	return nil
}
