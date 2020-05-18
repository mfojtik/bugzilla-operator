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

func Run(ctx context.Context, operatorConfig config.OperatorConfig) error {
	slackClient := slack.NewClient(operatorConfig.SlackChannel, operatorConfig.Credentials.DecodedSlackToken())
	recorder := slack.NewRecorder(slackClient, "BugzillaOperator", operatorConfig.SlackUserEmail)

	recorder.Eventf("OperatorStarted", "Bugzilla Operator Started\n\n```\n%s\n```\n", spew.Sdump(operatorConfig.Anonymize()))

	// stale controller marks bugs that are stale (unchanged for 30 days)
	staleController := stalecontroller.NewStaleController(operatorConfig, slackClient, recorder)

	// blocker bugs report nag people about their blocker bugs every second week between Tue->Thur
	blockerReportSchedule := informer.NewTimeInformer("blocker-bugs")
	blockerReportSchedule.Schedule("CRON_TZ=Europe/Prague 30 9 1-7,16-23 * 2-4")
	blockerReportSchedule.Schedule("CRON_TZ=America/New_York 30 9 1-7,16-23 * 2-4")
	blockerReporter := blockers.NewBlockersReporter(operatorConfig, blockerReportSchedule, slackClient, recorder)

	// closed bugs report post statistic about closed bugs to status channel in 24h between Mon->Fri
	closedReportSchedule := informer.NewTimeInformer("closed-bugs")
	closedReportSchedule.Schedule("CRON_TZ=Europe/Prague 30 9 * * 1-5")
	closedReportSchedule.Schedule("CRON_TZ=America/New_York 30 9 * * 1-5")
	closedReporter := closed.NewClosedReporter(operatorConfig, closedReportSchedule, slackClient, recorder)

	go blockerReportSchedule.Start(ctx)
	go blockerReporter.Run(ctx, 1)
	go closedReportSchedule.Start(ctx)
	go closedReporter.Run(ctx, 1)
	go staleController.Run(ctx, 1)

	<-ctx.Done()
	return nil
}
