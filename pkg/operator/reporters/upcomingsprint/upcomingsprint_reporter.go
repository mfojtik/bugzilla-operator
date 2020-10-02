package upcomingsprint

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/eparis/bugzilla"
	"github.com/openshift/library-go/pkg/controller/factory"
	"github.com/openshift/library-go/pkg/operator/events"

	"github.com/mfojtik/bugzilla-operator/pkg/cache"
	"github.com/mfojtik/bugzilla-operator/pkg/operator/config"
	"github.com/mfojtik/bugzilla-operator/pkg/operator/controller"
)

type UpcomingSprintReporter struct {
	controller.ControllerContext
	config     config.OperatorConfig
	components []string
}

func NewUpcomingSprintReporter(ctx controller.ControllerContext, components []string, schedule []string, operatorConfig config.OperatorConfig, recorder events.Recorder) factory.Controller {
	c := &UpcomingSprintReporter{
		ControllerContext: ctx,
		config:            operatorConfig,
		components:        components,
	}
	return factory.New().WithSync(c.sync).ResyncSchedule(schedule...).ToController("UpcomingSprintReporter", recorder)
}

func (c *UpcomingSprintReporter) sync(ctx context.Context, syncCtx factory.SyncContext) error {
	client := c.NewBugzillaClient(ctx)
	slackClient := c.SlackClient(ctx)

	report, err := Report(ctx, client, syncCtx.Recorder(), &c.config, c.components)
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

func makeBugLink(email string) string {
	return "https://bugzilla.redhat.com/buglist.cgi?bug_status=NEW&bug_status=ASSIGNED&bug_status=POST&bug_status=ON_DEV&" +
		"component=apiserver-auth&component=config-operator&component=Etcd&component=Etcd%20Operator" +
		"&component=kube-apiserver&component=kube-controller-manager&component=kube-scheduler&component=kube-storage-version-migrator&component=Master&" +
		"component=oauth-apiserver&component=oauth-proxy&component=oc" +
		"&component=openshift-apiserver&component=service-ca&email1=" + email + "&emailassigned_to1=1&emailtype1=substring&f4=" +
		"keywords&f5=status_whiteboard&" +
		"&o4=notsubstring&o5=notsubstring&product=OpenShift%20Container%20Platform&query_format=advanced&v4=UpcomingSprint&v5=LifecycleStale"
}

type sortedAssignee struct {
	assigneeName string
	bugCount     int
}

func Report(ctx context.Context, client cache.BugzillaClient, recorder events.Recorder, config *config.OperatorConfig, components []string) (string, error) {
	needUpcomingSprintBugs, err := getUpcomingSprintList(client, config, components)
	if err != nil {
		recorder.Warningf("BugSearchFailed", err.Error())
		return "", err
	}

	bugCounts := []*sortedAssignee{}
	for _, b := range needUpcomingSprintBugs {
		for _, n := range bugCounts {
			if n.assigneeName == b.AssignedTo {
				n.bugCount++
				continue
			}
		}
		bugCounts = append(bugCounts, &sortedAssignee{
			assigneeName: b.AssignedTo,
			bugCount:     1,
		})
	}
	sort.Slice(bugCounts, func(i, j int) bool {
		return bugCounts[i].bugCount <= bugCounts[j].bugCount
	})

	result := []string{
		"**Bugs without UpcomingSprint**\n",
	}
	for _, c := range bugCounts {
		warnSign := ":warning: "
		if c.bugCount < 10 {
			warnSign = ""
		}
		result = append(result, fmt.Sprintf("> %s%s: <%s|%d>", warnSign, c.assigneeName, makeBugLink(c.assigneeName), c.bugCount))
	}

	return strings.Join(result, "\n"), nil
}

func getUpcomingSprintList(client cache.BugzillaClient, config *config.OperatorConfig, components []string) ([]*bugzilla.Bug, error) {
	return client.Search(bugzilla.Query{
		Classification: []string{"Red Hat"},
		Product:        []string{"OpenShift Container Platform"},
		Status:         []string{"NEW", "ASSIGNED", "POST", "ON_DEV"},
		Component:      components,
		Advanced: []bugzilla.AdvancedQuery{
			{
				Field: "keywords",
				Op:    "notsubstring",
				Value: "UpcomingSprint",
			},
			{
				Field: "status_whiteboard",
				Op:    "notsubstring",
				Value: "LifecycleStale",
			},
		},
		IncludeFields: []string{
			"id",
			"assigned_to",
			"keywords",
			"status",
			"resolution",
			"severity",
			"priority",
			"target_release",
			"whiteboard",
		},
	})
}
