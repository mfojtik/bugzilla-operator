package operator

import (
	"context"

	"github.com/davecgh/go-spew/spew"

	"github.com/mfojtik/bugzilla-operator/pkg/operator/config"
	"github.com/mfojtik/bugzilla-operator/pkg/operator/reporters/blockers"
	"github.com/mfojtik/bugzilla-operator/pkg/operator/reporters/closed"
	"github.com/mfojtik/bugzilla-operator/pkg/operator/stalecontroller"
	"github.com/mfojtik/bugzilla-operator/pkg/slack"
)

func anonymizeConfig(in *config.OperatorConfig) config.OperatorConfig {
	out := *in
	out.Credentials = config.Credentials{}
	return out
}

func Run(ctx context.Context, operatorConfig config.OperatorConfig) error {
	slackClient := slack.NewClient(operatorConfig.SlackChannel, operatorConfig.Credentials.DecodedSlackToken())
	recorder := slack.NewRecorder(slackClient, "BugzillaOperator", operatorConfig.SlackUserEmail)

	recorder.Eventf("OperatorStarted", "Bugzilla Operator Started\n`%s`", spew.Sdump(anonymizeConfig(&operatorConfig)))

	staleController := stalecontroller.NewStaleController(operatorConfig, slackClient, recorder)
	go staleController.Run(ctx, 1)

	blockerReporter := blockers.NewBlockersReporter(operatorConfig, slackClient, recorder)
	go blockerReporter.Run(ctx, 1)

	closedReporter := closed.NewClosedReporter(operatorConfig, slackClient, recorder)
	go closedReporter.Run(ctx, 1)

	<-ctx.Done()
	return nil
}
