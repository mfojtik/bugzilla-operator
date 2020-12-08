package escalation

import (
	"context"
	"fmt"
	"strings"

	"github.com/mfojtik/bugzilla-operator/pkg/operator/bugutil"

	"github.com/mfojtik/bugzilla-operator/pkg/slack"
	"k8s.io/apimachinery/pkg/util/sets"

	"github.com/eparis/bugzilla"
	"github.com/openshift/library-go/pkg/controller/factory"
	"github.com/openshift/library-go/pkg/operator/events"

	"github.com/mfojtik/bugzilla-operator/pkg/cache"
	"github.com/mfojtik/bugzilla-operator/pkg/operator/config"
	"github.com/mfojtik/bugzilla-operator/pkg/operator/controller"
)

type EscalationReporter struct {
	controller.ControllerContext
	config     config.OperatorConfig
	components []string
}

func NewEscalationReporter(ctx controller.ControllerContext, components []string, schedule []string, operatorConfig config.OperatorConfig, recorder events.Recorder) factory.Controller {
	c := &EscalationReporter{
		ControllerContext: ctx,
		config:            operatorConfig,
		components:        components,
	}
	return factory.New().WithSync(c.sync).ResyncSchedule(schedule...).ToController("UrgentStatsReporter", recorder)
}

func (c *EscalationReporter) sync(ctx context.Context, syncCtx factory.SyncContext) error {
	client := c.NewBugzillaClient(ctx)
	slackClient := c.SlackClient(ctx)

	report, err := Report(ctx, client, slackClient, syncCtx.Recorder(), &c.config, c.components)
	if err != nil {
		return err
	}
	if len(report) == 0 {
		return nil
	}

	if err := slackClient.MessageChannel(report); err != nil {
		syncCtx.Recorder().Warningf("DeliveryFailed", "Failed to deliver closed bug counts: %v", err)
		return err
	}

	return nil
}

func Report(ctx context.Context, client cache.BugzillaClient, slack slack.ChannelClient, recorder events.Recorder, cfg *config.OperatorConfig, components []string) (string, error) {
	urgentSeverityBugs, err := getSeverityUrgentBugs(client, cfg, components)
	if err != nil {
		recorder.Warningf("BugSearchFailed", err.Error())
		return "", err
	}

	assigneeCounts := map[string][]*bugzilla.Bug{}
	leadsBugs := map[string][]*bugzilla.Bug{}
	missingComponents := sets.NewString()
	for _, b := range urgentSeverityBugs {
		escalated := b.Escalation == "Yes"
		customerCases := false
		for _, eb := range b.ExternalBugs {
			if eb.Type.Type == "SFDC" {
				customerCases = true
				break
			}
		}
		if escalated || (customerCases && b.Priority == "urgent") || (customerCases && b.Severity == "urgent" && b.Priority == "unspecified") {
			assigneeCounts[b.AssignedTo] = append(assigneeCounts[b.AssignedTo], b)

			if len(b.Component) > 0 {
				comp, ok := cfg.Components[b.Component[0]]
				if !ok {
					missingComponents.Insert(b.Component[0])
				}

				if len(comp.Lead) > 0 {
					leadsBugs[comp.Lead] = append(leadsBugs[comp.Lead], b)
				}
			}
		}
	}

	if len(assigneeCounts) == 0 {
		return "", nil
	}

	lines := []string{"Escalation report:"}
	for lead, bugs := range leadsBugs {
		roots := sets.NewString()
		for _, comp := range cfg.Components {
			if comp.Lead == lead {
				roots.Insert(comp.Developers...)
			}
		}
		team := config.ExpandGroups(cfg.Groups, roots.List()...)
		maxEscalations := max(1, int(float64(len(team))*0.2))

		if len(bugs) > maxEscalations {
			lines = append(lines, fmt.Sprintf(":red-siren: %s with %d bugs, above the quota of %d", lead, len(bugs), maxEscalations))
		} else {
			lines = append(lines, fmt.Sprintf("%s with %d bugs", lead, len(bugs)))
		}

		for _, b := range bugs {
			lines = append(lines, fmt.Sprintf("> %s %s @ %s: %s", bugutil.GetBugURL(*b), b.AssignedTo, b.Status, b.Summary))
		}
	}

	if len(missingComponents) > 0 && slack != nil {
		slack.MessageAdminChannel(fmt.Sprintf("Missing components in config: %s", strings.Join(missingComponents.List(), ", ")))
	}

	first := true
	for assignee, bugs := range assigneeCounts {
		if len(bugs) == 1 {
			continue
		}

		if first {
			lines = append(lines, "")
			lines = append(lines, "Assignees with more than one escalation:")
			first = false
		}

		links := []string{}
		for _, b := range bugs {
			links = append(links, bugutil.GetBugURL(*b))
		}

		lines = append(lines, fmt.Sprintf("> :red-siren: %s: %s", assignee, strings.Join(links, " ")))
	}

	return strings.Join(lines, "\n"), nil
}

func max(x, y int) int {
	if x > y {
		return x
	}
	return y
}

func getSeverityUrgentBugs(client cache.BugzillaClient, config *config.OperatorConfig, components []string) ([]*bugzilla.Bug, error) {
	return client.Search(bugzilla.Query{
		Classification: []string{"Red Hat"},
		Product:        []string{"OpenShift Container Platform"},
		Status:         []string{"NEW", "ASSIGNED", "POST", "ON_DEV"},
		Component:      components,
		IncludeFields: []string{
			"id",
			"assigned_to",
			"status",
			"severity",
			"priority",
			"external_bugs",
			"component",
			"summary",
		},
	})
}
