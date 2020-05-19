package operator

import (
	"context"

	"github.com/davecgh/go-spew/spew"
	slackgo "github.com/slack-go/slack"

	"github.com/mfojtik/bugzilla-operator/pkg/operator/closecontroller"
	"github.com/mfojtik/bugzilla-operator/pkg/operator/config"
	"github.com/mfojtik/bugzilla-operator/pkg/operator/reporters/blockers"
	"github.com/mfojtik/bugzilla-operator/pkg/operator/reporters/closed"
	"github.com/mfojtik/bugzilla-operator/pkg/operator/reporters/informer"
	"github.com/mfojtik/bugzilla-operator/pkg/operator/stalecontroller"
	"github.com/mfojtik/bugzilla-operator/pkg/slack"
	"github.com/mfojtik/bugzilla-operator/pkg/slacker"
)

func Run(ctx context.Context, operatorConfig config.OperatorConfig) error {
	slackClient := slackgo.New(operatorConfig.Credentials.DecodedSlackToken(), slackgo.OptionDebug(true))
	slackChannelClient := slack.NewChannelClient(slackClient, operatorConfig.SlackChannel)
	recorder := slack.NewRecorder(slackChannelClient, "BugzillaOperator", operatorConfig.SlackUserEmail)

	slackerInstance := slacker.NewSlacker(slackClient, slacker.Options{
		ListenAddress:     "0.0.0.0:3000",
		VerificationToken: operatorConfig.Credentials.DecodedSlackVerificationToken(),
	})
	slackerInstance.Command("say <message>", &slacker.CommandDefinition{
		Description: "Say something.",
		Handler: func(req slacker.Request, w slacker.ResponseWriter) {
			msg := req.StringParam("message", "")
			w.Reply(msg)
		},
	})
	slackerInstance.DefaultCommand(func(req slacker.Request, w slacker.ResponseWriter) {
		w.Reply("Unknown command")
	})

	recorder.Eventf("OperatorStarted", "Bugzilla Operator Started\n\n```\n%s\n```\n", spew.Sdump(operatorConfig.Anonymize()))

	// stale controller marks bugs that are stale (unchanged for 30 days)
	staleController := stalecontroller.NewStaleController(operatorConfig, slackChannelClient, recorder)

	// close stale controller automatically close bugs that were not updated after marked LifecycleClose for 7 days
	closeStaleController := closecontroller.NewCloseStaleController(operatorConfig, slackChannelClient, recorder)

	// blocker bugs report nag people about their blocker bugs every second week between Tue->Thur
	blockerReportSchedule := informer.NewTimeInformer("blocker-bugs")

	blockerReportSchedule.Schedule("CRON_TZ=Europe/Prague 30 9 1-7,16-23 * 2-4")
	blockerReportSchedule.Schedule("CRON_TZ=America/New_York 30 9 1-7,16-23 * 2-4")
	blockerReporter := blockers.NewBlockersReporter(operatorConfig, blockerReportSchedule, slackChannelClient, recorder)

	// closed bugs report post statistic about closed bugs to status channel in 24h between Mon->Fri
	closedReportSchedule := informer.NewTimeInformer("closed-bugs")
	closedReportSchedule.Schedule("CRON_TZ=Europe/Prague 30 9 * * 1-5")
	closedReportSchedule.Schedule("CRON_TZ=America/New_York 30 9 * * 1-5")
	closedReporter := closed.NewClosedReporter(operatorConfig, closedReportSchedule, slackChannelClient, recorder)

	// report command allow to manually trigger a reporter to run out of its normal schedule
	slackerInstance.Command("report <job>", &slacker.CommandDefinition{
		Description: "Trigger a job to run.",
		Handler: func(req slacker.Request, w slacker.ResponseWriter) {
			msg := req.StringParam("job", "")
			switch msg {
			case "blocker-bugs":
				blockerReportSchedule.RunNow()
			case "closed-bugs":
				closedReportSchedule.RunNow()
			}
		},
	})

	go blockerReportSchedule.Start(ctx)
	go blockerReporter.Run(ctx, 1)
	go closedReportSchedule.Start(ctx)
	go closedReporter.Run(ctx, 1)
	go staleController.Run(ctx, 1)
	go closeStaleController.Run(ctx, 1)
	go slackerInstance.Run(ctx)

	<-ctx.Done()
	return nil
}
