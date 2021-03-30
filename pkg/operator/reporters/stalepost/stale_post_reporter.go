package stalepost

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/eparis/bugzilla"
	"github.com/google/go-github/v33/github"
	"github.com/openshift/library-go/pkg/controller/factory"
	"github.com/openshift/library-go/pkg/operator/events"
	"golang.org/x/oauth2"
	errutil "k8s.io/apimachinery/pkg/util/errors"

	"github.com/mfojtik/bugzilla-operator/pkg/cache"
	"github.com/mfojtik/bugzilla-operator/pkg/operator/bugutil"
	"github.com/mfojtik/bugzilla-operator/pkg/operator/config"
	"github.com/mfojtik/bugzilla-operator/pkg/operator/controller"
)

// StalePostReporter monitor bugs that are in POST state and report them.
type StalePostReporter struct {
	controller.ControllerContext
	config config.OperatorConfig
}

func NewStalePostReporter(ctx controller.ControllerContext, schedule []string, operatorConfig config.OperatorConfig, recorder events.Recorder) factory.Controller {
	c := &StalePostReporter{ControllerContext: ctx, config: operatorConfig}
	return factory.New().WithSync(c.sync).ResyncSchedule(schedule...).ToController("StalePostReporter", recorder)
}

func (c *StalePostReporter) sync(ctx context.Context, syncCtx factory.SyncContext) (err error) {
	client := c.NewBugzillaClient(ctx)
	slackClient := c.SlackClient(ctx)
	report, err := Report(ctx, client, &c.config)
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

func Report(ctx context.Context, client cache.BugzillaClient, config *config.OperatorConfig) (string, error) {
	ghClient := github.NewClient(oauth2.NewClient(ctx, oauth2.StaticTokenSource(&oauth2.Token{AccessToken: config.GithubToken})))
	bugs, err := getNewBugs(client, config)
	if err != nil {
		return "", err
	}

	var errors []error

	result := []string{
		fmt.Sprintf("Found %d bugs in POST state for longer than 3 days:\n", len(bugs)),
	}

	for _, b := range bugs {
		result = append(result, bugutil.FormatBugMessage(*b))
		for _, e := range b.ExternalBugs {
			if e.Type.Type != "GitHub" {
				continue
			}
			pr, err := getGithubPullFromExternalBugID(ctx, ghClient, e.ExternalBugID)
			if err != nil {
				errors = append(errors, fmt.Errorf("bug #%d querying github failed: %v", b.ID, err))
				continue
			}
			result = append(result, fmt.Sprintf(">   :pull-request: [%s] <%s|%s> %s", pr.GetBase().GetRef(), pr.GetHTMLURL(), pr.GetTitle(), formatPullRequestLabels(pr.Labels)))
		}
	}

	return strings.Join(result, "\n"), errutil.NewAggregate(errors)
}

func formatPullRequestLabels(labels []*github.Label) string {
	var result []string
	isLgtm := false
	isApproved := false
	isOnHold := false
	for _, l := range labels {
		if l == nil {
			continue
		}
		switch l.GetName() {
		case "lgtm":
			result = append(result, fmt.Sprintf(":lgtm:"))
			isLgtm = true
		case "approved":
			result = append(result, fmt.Sprintf(":approved:"))
			isApproved = true
		case "cherry-pick-approved":
			result = append(result, fmt.Sprintf(":rocket:"))
		case "do-not-merge/hold":
			isOnHold = true
		}
	}
	if !isLgtm {
		result = append(result, fmt.Sprintf(":warning: *waiting for lgtm*"))
	}
	if !isApproved {
		result = append(result, fmt.Sprintf(":warning: *waiting for approval*"))
	}
	if isOnHold {
		result = append(result, fmt.Sprintf(":warning: *on hold*"))
	}
	return strings.Join(result, " ")
}

func getGithubPullFromExternalBugID(ctx context.Context, ghClient *github.Client, externalBugID string) (*github.PullRequest, error) {
	// format: openshift/openshift-apiserver/pull/188
	parts := strings.Split(externalBugID, "/")
	if len(parts) != 4 {
		return nil, fmt.Errorf("wrong pull request format in external bug ID")
	}
	ghNumber, err := strconv.Atoi(parts[3])
	if err != nil {
		return nil, err
	}
	pr, _, err := ghClient.PullRequests.Get(ctx, parts[0], parts[1], ghNumber)
	if err != nil {
		return nil, err
	}
	return pr, nil
}

func getNewBugs(client cache.BugzillaClient, c *config.OperatorConfig) ([]*bugzilla.Bug, error) {
	aq := bugzilla.AdvancedQuery{
		Field: "days_elapsed",
		Op:    "greaterthan",
		Value: "3",
	}
	return client.Search(bugzilla.Query{
		Classification: []string{"Red Hat"},
		Product:        []string{"OpenShift Container Platform"},
		Status:         []string{"POST"},
		Severity:       []string{"urgent", "high"},
		Component:      c.Components.List(),
		Advanced:       []bugzilla.AdvancedQuery{aq},
		IncludeFields: []string{
			"id",
			"assigned_to",
			"component",
			"severity",
			"priority",
			"summary",
			"status",
			"external_bugs",
		},
	})
}
