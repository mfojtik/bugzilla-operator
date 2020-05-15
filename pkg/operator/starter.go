package operator

import (
	"context"

	"github.com/davecgh/go-spew/spew"

	"github.com/mfojtik/bugzilla-operator/pkg/operator/config"
	"github.com/mfojtik/bugzilla-operator/pkg/operator/reporters/blockers"
	"github.com/mfojtik/bugzilla-operator/pkg/operator/reporters/closed"
	"github.com/mfojtik/bugzilla-operator/pkg/operator/reporters/informer"
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

	blockerReportSchedule := informer.NewTimeInformer()
	blockerReportSchedule.Schedule("CRON_TZ=Europe/Prague 30 9 1-7,16-23 * 2-4")
	blockerReportSchedule.Schedule("CRON_TZ=America/Boston 30 9 1-7,16-23 * 2-4")
	go blockerReportSchedule.Start(ctx)

	blockerReporter := blockers.NewBlockersReporter(operatorConfig, blockerReportSchedule, slackClient, recorder)
	go blockerReporter.Run(ctx, 1)

	closedReportSchedule := informer.NewTimeInformer()
	closedReportSchedule.Schedule("CRON_TZ=Europe/Prague 30 9 * * 1-5")
	closedReportSchedule.Schedule("CRON_TZ=Europe/Boston 30 9 * * 1-5")
	go closedReportSchedule.Start(ctx)

	closedReporter := closed.NewClosedReporter(operatorConfig, closedReportSchedule, slackClient, recorder)
	go closedReporter.Run(ctx, 1)

	<-ctx.Done()
	return nil
}
