package operator

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/davecgh/go-spew/spew"
	"github.com/eparis/bugzilla"
	"github.com/openshift/library-go/pkg/controller/factory"
	slackgo "github.com/slack-go/slack"
	"k8s.io/klog"

	"github.com/mfojtik/bugzilla-operator/pkg/cache"
	"github.com/mfojtik/bugzilla-operator/pkg/operator/closecontroller"
	"github.com/mfojtik/bugzilla-operator/pkg/operator/config"
	"github.com/mfojtik/bugzilla-operator/pkg/operator/reporters/blockers"
	"github.com/mfojtik/bugzilla-operator/pkg/operator/reporters/closed"
	"github.com/mfojtik/bugzilla-operator/pkg/operator/resetcontroller"
	"github.com/mfojtik/bugzilla-operator/pkg/operator/stalecontroller"
	"github.com/mfojtik/bugzilla-operator/pkg/slack"
	"github.com/mfojtik/bugzilla-operator/pkg/slacker"
)

const bugzillaEndpoint = "https://bugzilla.redhat.com"

func Run(ctx context.Context, cfg config.OperatorConfig) error {
	if len(cfg.CachePath) > 0 {
		cache.Open(cfg.CachePath)
	}
	defer cache.Close()

	slackClient := slackgo.New(cfg.Credentials.DecodedSlackToken(), slackgo.OptionDebug(true))

	// This slack client is used for production notifications
	// Be careful, this can spam people!
	slackProductionClient := slack.NewChannelClient(slackClient, cfg.SlackChannel, false)

	// This slack client is used for debugging
	slackDebugClient := slack.NewChannelClient(slackClient, cfg.SlackAdminChannel, true)

	recorder := slack.NewRecorder(slackDebugClient, "BugzillaOperator")

	slackerInstance := slacker.NewSlacker(slackClient, slacker.Options{
		ListenAddress:     "0.0.0.0:3000",
		VerificationToken: cfg.Credentials.DecodedSlackVerificationToken(),
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

	recorder.Eventf("OperatorStarted", "Bugzilla Operator Started\n\n```\n%s\n```\n", spew.Sdump(cfg.Anonymize()))

	// stale controller marks bugs that are stale (unchanged for 30 days)
	staleController := stalecontroller.NewStaleController(cfg, newBugzillaClient(&cfg), slackProductionClient, recorder)

	staleResetController := resetcontroller.NewResetStaleController(cfg, newBugzillaClient(&cfg), slackProductionClient, slackDebugClient, recorder)

	// close stale controller automatically close bugs that were not updated after marked LifecycleClose for 7 days
	closeStaleController := closecontroller.NewCloseStaleController(cfg, newBugzillaClient(&cfg), slackProductionClient, slackDebugClient, recorder)

	blockerReporter := blockers.NewBlockersReporter([]string{
		"CRON_TZ=Europe/Prague 30 9 1-7,16-23 * 2-4",
		"CRON_TZ=America/New_York 30 9 1-7,16-23 * 2-4",
	}, cfg, newBugzillaClient(&cfg), slackProductionClient, slackDebugClient, recorder)

	// closed bugs report post statistic about closed bugs to status channel in 24h between Mon->Fri
	closedReporter := closed.NewClosedReporter([]string{
		"CRON_TZ=Europe/Prague 35 9 * * 1-5",
		"CRON_TZ=America/New_York 35 9 * * 1-5",
	}, cfg, newBugzillaClient(&cfg), slackProductionClient, recorder)

	// report command allow to manually trigger a reporter to run out of its normal schedule
	slackerInstance.Command("admin trigger <job>", &slacker.CommandDefinition{
		Description: "Trigger a job to run.",
		Handler: auth(cfg, func(req slacker.Request, w slacker.ResponseWriter) {
			job := req.StringParam("job", "")

			reports := map[string]func(ctx context.Context, controllerContext factory.SyncContext) error{
				"blocker-bugs": blockerReporter.Sync,
				"closed-bugs":  closedReporter.Sync,

				// don't forget to also add new reports down in the direct report command
			}

			switch job {
			case "help", "":
				names := []string{}
				for s := range reports {
					names = append(names, s)
				}
				sort.Strings(names)
				w.Reply(strings.Join(names, "\n"))
			default:
				if report, ok := reports[job]; ok {
					if err := report(ctx, factory.NewSyncContext(job, recorder)); err != nil {
						recorder.Warningf("ReportError", "Job reported error: %v", err)
						return
					}
					_, _, _, err := w.Client().SendMessage(req.Event().Channel,
						slackgo.MsgOptionPostEphemeral(req.Event().User),
						slackgo.MsgOptionText(fmt.Sprintf("Triggered job %q", job), false))
					if err != nil {
						klog.Error(err)
					}
				} else {
					w.Reply(fmt.Sprintf("Unknown report %q", job))
				}
			}
		}, "group:admins"),
	})
	slackerInstance.Command("report <job>", &slacker.CommandDefinition{
		Description: "Run a report and print result here.",
		Handler: func(req slacker.Request, w slacker.ResponseWriter) {
			job := req.StringParam("job", "")
			reports := map[string]func(ctx context.Context, client cache.BugzillaClient) (string, error){
				"blocker-bugs": func(ctx context.Context, client cache.BugzillaClient) (string, error) {
					report, _, err := blockers.Report(ctx, client, recorder, &cfg)
					return report, err
				},
				"closed-bugs": func(ctx context.Context, client cache.BugzillaClient) (string, error) {
					return closed.Report(ctx, client, recorder, &cfg)
				},

				// don't forget to also add new reports above in the trigger command
			}

			switch job {
			case "help", "":
				names := []string{}
				for s := range reports {
					names = append(names, s)
				}
				sort.Strings(names)
				w.Reply(strings.Join(names, "\n"))
			default:
				report, ok := reports[job]
				if !ok {
					w.Reply(fmt.Sprintf("Unknown report %q", job))
					break
				}

				_, _, _, err := w.Client().SendMessage(req.Event().Channel,
					slackgo.MsgOptionPostEphemeral(req.Event().User),
					slackgo.MsgOptionText(fmt.Sprintf("Running job %q. This might take some seconds.", job), false))
				if err != nil {
					klog.Error(err)
				}

				reply, err := report(context.TODO(), newBugzillaClient(&cfg)())
				if err != nil {
					_, _, _, err := w.Client().SendMessage(req.Event().Channel,
						slackgo.MsgOptionPostEphemeral(req.Event().User),
						slackgo.MsgOptionText(fmt.Sprintf("Error running report %v: %v", job, err), false))
					if err != nil {
						klog.Error(err)
					}
				} else {
					w.Reply(reply)
				}
			}
		},
	})

	go blockerReporter.Run(ctx, 1)
	go closedReporter.Run(ctx, 1)
	go staleController.Run(ctx, 1)
	go staleResetController.Run(ctx, 1)
	go closeStaleController.Run(ctx, 1)
	go slackerInstance.Run(ctx)

	<-ctx.Done()
	return nil
}

func newBugzillaClient(cfg *config.OperatorConfig) func() cache.BugzillaClient {
	return func() cache.BugzillaClient {
		return cache.NewCachedBugzillaClient(bugzilla.NewClient(func() []byte {
			return []byte(cfg.Credentials.DecodedAPIKey())
		}, bugzillaEndpoint).WithCGIClient(cfg.Credentials.DecodedUsername(), cfg.Credentials.DecodedPassword()))
	}
}
