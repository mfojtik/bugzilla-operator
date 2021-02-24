package escalation

import (
	"context"
	"fmt"
	"net/url"
	"sort"
	"strings"
	"time"

	humanize "github.com/dustin/go-humanize"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/klog"

	"github.com/eparis/bugzilla"
	"github.com/openshift/library-go/pkg/controller/factory"
	"github.com/openshift/library-go/pkg/operator/events"

	"github.com/mfojtik/bugzilla-operator/pkg/cache"
	"github.com/mfojtik/bugzilla-operator/pkg/operator/bugutil"
	"github.com/mfojtik/bugzilla-operator/pkg/operator/config"
	"github.com/mfojtik/bugzilla-operator/pkg/operator/controller"
	"github.com/mfojtik/bugzilla-operator/pkg/slack"
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

	report, componentStats, err := Report(ctx, client, slackClient, syncCtx.Recorder(), &c.config, c.components)
	if err != nil {
		return err
	}

	// print group report
	if len(report) > 0 {
		if err := slackClient.MessageChannel(report); err != nil {
			syncCtx.Recorder().Warningf("DeliveryFailed", "Failed to deliver closed bug counts: %v", err)
			return err
		}
	}

	// send component stats to admin channel
	lines := []string{"Component escalation stats:"}
	ordered := []string{}
	for comp := range componentStats {
		ordered = append(ordered, comp)
	}
	sort.Slice(ordered, func(i, j int) bool {
		return len(componentStats[ordered[i]]) > len(componentStats[ordered[j]])
	})
	for _, comp := range ordered {
		bugs := componentStats[comp]
		ids := []int{}
		for _, b := range bugs {
			ids = append(ids, b.ID)
		}
		line := fmt.Sprintf("- %s: %d", makeBugzillaLink(comp, ids...), len(bugs))
		lines = append(lines, line)
	}
	if err := slackClient.MessageAdminChannel(strings.Join(lines, "\n")); err != nil {
		return err
	}

	return nil
}

func Report(ctx context.Context, client cache.BugzillaClient, slack slack.ChannelClient, recorder events.Recorder, cfg *config.OperatorConfig, components []string) (string, map[string][]*bugzilla.Bug, error) {
	ourComponents := sets.NewString(components...)
	urgentSeverityBugs, err := getSeverityUrgentBugs(client, cfg)
	if err != nil {
		recorder.Warningf("BugSearchFailed", err.Error())
		return "", nil, err
	}

	assigned := map[string][]*bugzilla.Bug{}
	silenced := []*bugzilla.Bug{}
	leadsBugs := map[string][]*bugzilla.Bug{}
	componentStats := map[string][]*bugzilla.Bug{}
	questionable := map[int]bool{}
	missingComponents := sets.NewString()
	for _, b := range urgentSeverityBugs {
		escalationFlag := b.Escalation == "Yes"
		customerCases := false
		highOrUrgent := false
		for _, eb := range b.ExternalBugs {
			if eb.Type.Type == "SFDC" && eb.ExternalStatus != "Closed" {
				customerCases = true

				if prio := strings.ToLower(eb.ExternalPriority); strings.Contains(prio, "high") || strings.Contains(prio, "urgent") {
					highOrUrgent = true
				}
			}
		}

		ours := false
		for _, c := range b.Component {
			if ourComponents.Has(c) {
				ours = true
				break
			}
		}

		isEscalation := (escalationFlag && (b.Severity == "urgent" || b.Priority == "urgent")) ||
			(customerCases && b.Priority == "urgent") ||
			(customerCases && b.Severity == "urgent" && b.Priority == "unspecified")
		isSilenced := customerCases && b.Severity == "urgent" && b.Priority != "urgent"

		if isEscalation {
			for _, c := range b.Component {
				componentStats[c] = append(componentStats[c], b)
			}
		}

		if !ours {
			continue
		}

		if isEscalation {
			assigned[b.AssignedTo] = append(assigned[b.AssignedTo], b)

			comp, ok := cfg.Components[b.Component[0]]
			if !ok {
				missingComponents.Insert(b.Component[0])
			}

			if len(comp.Lead) > 0 {
				leadsBugs[comp.Lead] = append(leadsBugs[comp.Lead], b)
			}

			if !highOrUrgent {
				questionable[b.ID] = true
			}
		} else if isSilenced {
			silenced = append(silenced, b)
		}
	}

	if len(missingComponents) > 0 && slack != nil {
		slack.MessageAdminChannel(fmt.Sprintf("Missing components in config: %s", strings.Join(missingComponents.List(), ", ")))
	}

	if len(leadsBugs) == 0 && len(silenced) == 0 {
		return "", componentStats, nil
	}

	lines := []string{"Escalation report:", ""}

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
			lines = append(lines, fmt.Sprintf(":red-siren: %s's team with %d bugs, above the quota of %d", lead, len(bugs), maxEscalations))
		} else {
			lines = append(lines, fmt.Sprintf("%s's team with %d bug", lead, len(bugs)))
		}

		for _, b := range bugs {
			ageString := ""
			lastChanged, err := time.Parse(time.RFC3339, b.LastChangeTime)
			if err != nil {
				klog.Warningf("Cannot parse last-change-time %q of #%d: %v", b.LastChangeTime, b.ID, err)
			} else {
				ageString = fmt.Sprintf(", changed *%s*", humanize.Time(lastChanged))
			}

			versionString := ""
			if v := bugutil.FormatVersion(b.TargetRelease); v != "---" {
				versionString = fmt.Sprintf(", *%s*", v)
			}

			line := fmt.Sprintf("> %s [*%s* in *%s*%s%s] @ %s: %s", bugutil.GetBugURL(*b), b.Status, bugutil.FormatComponent(b.Component), versionString, ageString, b.AssignedTo, b.Summary)

			warnings := []string{}
			if questionable[b.ID] {
				warnings = append(warnings, "no high/urgent customer case, or closed")
			}

			if !lastChanged.IsZero() {
				max := time.Hour * 48
				switch time.Now().Weekday() {
				case time.Monday, time.Tuesday:
					max += time.Hour * 48
				}

				if lastChanged.Before(time.Now().Add(-max)) {
					needInfoFromReporter := false
					needInfoFromUs := false
					for _, f := range b.Flags {
						if f.Name == "needinfo" && f.Status == "?" {
							if f.Requestee == b.AssignedTo {
								needInfoFromUs = true
							} else {
								needInfoFromReporter = true
							}
						}
					}

					if needInfoFromUs {
						warnings = append(warnings, fmt.Sprintf("`needinfo?` from us older than 48h"))
					} else if needInfoFromReporter {
						warnings = append(warnings, fmt.Sprintf("`needinfo?` from reporter older than 48h"))
					}
				}
			}

			if len(warnings) > 0 {
				line = fmt.Sprintf("%s  — :warning: %s. Double check!", line, strings.Join(warnings, ". "))
			}

			lines = append(lines, line)
		}
	}

	first := true
	for assignee, bugs := range assigned {
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

	if len(silenced) > 0 {
		links := []string{}
		for _, b := range silenced {
			links = append(links, bugutil.GetBugURL(*b))
		}
		lines = append(lines, "", fmt.Sprintf("%d silenced bugs :see_no_evil: : %s", len(links), strings.Join(links, " ")))
	}

	return strings.Join(lines, "\n"), componentStats, nil
}

func max(x, y int) int {
	if x > y {
		return x
	}
	return y
}

func getSeverityUrgentBugs(client cache.BugzillaClient, config *config.OperatorConfig) ([]*bugzilla.Bug, error) {
	return client.Search(bugzilla.Query{
		Classification: []string{"Red Hat"},
		Product:        []string{"OpenShift Container Platform"},
		Status:         []string{"NEW", "ASSIGNED", "POST", "ON_DEV", "MODIFIED", "ON_QA", "RELEASE_PENDING"},
		IncludeFields: []string{
			"id",
			"assigned_to",
			"status",
			"severity",
			"priority",
			"external_bugs",
			"component",
			"summary",
			"cf_cust_facing",
			"target_release",
			"flags",
			"last_change_time",
			"reporter",
		},
	})
}

func makeBugzillaLink(hrefText string, ids ...int) string {
	u, _ := url.Parse("https://bugzilla.redhat.com/buglist.cgi?f1=bug_id&list_id=11100046&o1=anyexact&query_format=advanced")
	e := u.Query()
	stringIds := make([]string, len(ids))
	for i := range stringIds {
		stringIds[i] = fmt.Sprintf("%d", ids[i])
	}
	e.Add("v1", strings.Join(stringIds, ","))
	u.RawQuery = e.Encode()
	return fmt.Sprintf("<%s|%s>", u.String(), hrefText)
}
