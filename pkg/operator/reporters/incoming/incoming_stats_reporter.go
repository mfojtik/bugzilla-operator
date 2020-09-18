package incoming

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/mfojtik/bugzilla-operator/pkg/operator/config"
	"github.com/mfojtik/bugzilla-operator/pkg/operator/controller"
	"github.com/openshift/library-go/pkg/controller/factory"
	"github.com/openshift/library-go/pkg/operator/events"
)

type IncomingStatsReporter struct {
	controller.ControllerContext
	config config.OperatorConfig
}

func ReportStats(ctx context.Context, controllerCtx controller.ControllerContext, recorder events.Recorder, config *config.OperatorConfig) (string, error) {
	reportsJSON, err := controllerCtx.GetPersistentValue(ctx, "incoming-report")
	if err != nil {
		recorder.Warningf("GetPersistentValueFailed", "Failed to get incoming-report: %v", err)
		return "", err
	}
	reports, err := incomingReportFromJSONString(reportsJSON)
	if err != nil {
		recorder.Warningf("DecodeIncomingReportFailed", "Failed to decode incoming-report: %v", err)
		return "", err
	}

	curWeekReports := []IncomingDailyReport{}
	prevWeekReports := []IncomingDailyReport{}

	for _, r := range reports.Reports {
		if r.Timestamp.After(time.Now().Add(-7 * 24 * time.Hour)) {
			curWeekReports = append(curWeekReports, r)
			continue
		}
		if r.Timestamp.After(time.Now().Add(-14*24*time.Hour)) && r.Timestamp.Before(time.Now().Add(-7*24*time.Hour)) {
			prevWeekReports = append(prevWeekReports, r)
			continue
		}
	}

	curWeekComponents, curWeekSeverities := reportsToMap(curWeekReports)
	prevWeekComponents, prevWeekSeverities := reportsToMap(prevWeekReports)

	slackMessages := []string{"*Last Week - Bug Incoming Rates per Component:* "}
	slackMessages = append(slackMessages, reportToSlackMessages(curWeekComponents, prevWeekComponents)...)
	slackMessages = append(slackMessages, []string{"\n", "*Last Week - Bug Incoming Rates per Severity:* "}...)
	slackMessages = append(slackMessages, reportToSlackMessages(curWeekSeverities, prevWeekSeverities)...)

	return strings.Join(slackMessages, "\n"), nil
}

func (r *IncomingStatsReporter) sync(ctx context.Context, controllerContext factory.SyncContext) error {
	slackClient := r.SlackClient(ctx)
	message, err := ReportStats(ctx, r.ControllerContext, controllerContext.Recorder(), nil)
	if err != nil {
		return err
	}
	return slackClient.MessageAdminChannel(message)
}

func reportToSlackMessages(curReport, prevReport map[string]int) []string {
	slackMessages := []string{}
	for name, count := range curReport {
		prevWeekCount, ok := prevReport[name]
		prevWeekCountMessage := " (no bugs previous week)"
		if ok {
			switch {
			case prevWeekCount == count:
				prevWeekCountMessage = " (same as previous week)"
			case prevWeekCount > count:
				prevWeekCountMessage = fmt.Sprintf(" (:arrow_down: %d)", count-prevWeekCount)
			case prevWeekCount < count:
				prevWeekCountMessage = fmt.Sprintf(" (:arrow_up_small: %d)", count-prevWeekCount)
			}
		}
		slackMessages = append(slackMessages, fmt.Sprintf("_%s_: %d %s", name, count, prevWeekCountMessage))
	}
	return slackMessages
}

func reportsToMap(reports []IncomingDailyReport) (map[string]int, map[string]int) {
	components := map[string]int{}
	severities := map[string]int{}

	for i := range reports {
		for c := range reports[i].Components {
			components[reports[i].Components[c].Name] += reports[i].Components[c].Count
		}
		for c := range reports[i].Severities {
			severities[reports[i].Severities[c].Name] += reports[i].Severities[c].Count
		}
	}
	return components, severities
}

func NewIncomingStatsReporter(ctx controller.ControllerContext, schedule []string, operatorConfig config.OperatorConfig, recorder events.Recorder) factory.Controller {
	c := &IncomingStatsReporter{
		ControllerContext: ctx,
		config:            operatorConfig,
	}
	return factory.New().WithSync(c.sync).ResyncSchedule(schedule...).ToController("IncomingStatsReporter", recorder)
}
