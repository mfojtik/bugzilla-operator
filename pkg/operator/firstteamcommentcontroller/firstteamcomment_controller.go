package firstteamcommentcontroller

import (
	"context"
	"fmt"
	"net/url"
	"strings"
	"time"

	"github.com/eparis/bugzilla"
	"github.com/openshift/library-go/pkg/controller/factory"
	"github.com/openshift/library-go/pkg/operator/events"
	errutil "k8s.io/apimachinery/pkg/util/errors"
	"k8s.io/klog"

	"github.com/mfojtik/bugzilla-operator/pkg/cache"
	"github.com/mfojtik/bugzilla-operator/pkg/operator/config"
	"github.com/mfojtik/bugzilla-operator/pkg/slack"
)

type FirstTeamCommentController struct {
	config            config.OperatorConfig
	newBugzillaClient func() cache.BugzillaClient
	slackClient       slack.ChannelClient
}

func NewFirstTeamCommentController(operatorConfig config.OperatorConfig, newBugzillaClient func() cache.BugzillaClient, slackClient slack.ChannelClient, recorder events.Recorder) factory.Controller {
	c := &FirstTeamCommentController{
		config:            operatorConfig,
		newBugzillaClient: newBugzillaClient,
		slackClient:       slackClient,
	}
	return factory.New().WithSync(c.sync).ResyncEvery(2*time.Hour).ToController("FirstTeamCommentController", recorder)
}

func (c *FirstTeamCommentController) sync(ctx context.Context, syncCtx factory.SyncContext) error {
	client := c.newBugzillaClient()
	var errors []error

	for name, comp := range c.config.Components {
		if !comp.AssignFirstDeveloperCommentor {
			continue
		}
		if comp.Lead == "" {
			continue
		}

		nonLeads := config.ExpandGroups(c.config.Groups, comp.Developers...)
		nonLeads = nonLeads.Delete(comp.Lead)

		query := fmt.Sprintf(
			"email1=%s&email2=%s&emailassigned_to2=1&emaillongdesc1=1&emaillongdesc3=1&emailtype1=regexp&emailtype2=equals",
			url.QueryEscape(strings.Join(nonLeads.List(), "|")),
			url.QueryEscape(comp.Lead),
		)
		klog.Warning(query)
		leadAssignedBugs, err := client.Search(bugzilla.Query{
			Product:   []string{"OpenShift Container Platform"},
			Status:    []string{"NEW"},
			Component: []string{name},
			Raw:       query,
		})
		if err != nil {
			syncCtx.Recorder().Warningf("BuglistFailed", err.Error())
			continue
		}
		klog.Infof("%d NEW bugs found assigned to lead", len(leadAssignedBugs))

	nextBug:
		for _, b := range leadAssignedBugs {
			comments, err := client.GetCachedBugComments(b.ID, b.LastChangeTime)
			if err != nil {
				syncCtx.Recorder().Warningf("GetCachedBugComments", err.Error())
				continue
			}

			var firstTeamCommentor string
			for _, c := range comments {
				creator := c.Creator
				if !strings.ContainsRune(creator, '@') {
					creator = creator + "@redhat.com"
				}
				if strings.Contains(c.Text, "LifecycleStale") {
					continue
				}
				if nonLeads.Has(creator) && firstTeamCommentor == "" && b.Creator != creator {
					firstTeamCommentor = creator
				}
				if creator == comp.Lead {
					continue nextBug
				}
			}

			if firstTeamCommentor == "" {
				continue
			}

			history, err := client.GetCachedBugHistory(b.ID, b.LastChangeTime)
			if err != nil {
				syncCtx.Recorder().Warningf("GetBugFailed", err.Error())
				continue
			}

			for _, h := range history {
				for _, c := range h.Changes {
					if c.FieldName != "assigned_to" {
						continue
					}
					if c.Removed == comp.Lead {
						// this was bounced back before from lead. Don't try again.
						klog.Infof("Ignoring %v which was bounced from %s before.", b.ID, comp.Lead)
						continue nextBug
					}
					if c.Removed == firstTeamCommentor {
						// this was bounced back before from first commentor. Don't try again.
						klog.Infof("Ignoring %v which was bounced from %s before.", b.ID, firstTeamCommentor)
						continue nextBug
					}

				}
			}

			klog.Infof("%s commented on #%v, but lead %s hasn't.\n", firstTeamCommentor, b.ID, comp.Lead)
			c.slackClient.MessageChannel(fmt.Sprintf("FirstTeamCommentController would reassign %s bug <https://bugzilla.redhat.com/show_bug.cgi?id=%v|#%v %q> to %s due to comments, but lead %s hasn't commented.", name, b.ID, b.ID, b.Summary, firstTeamCommentor, comp.Lead))

			// TODO: enable bugzilla call
			/*
				if err := client.UpdateBug(b.ID, bugzilla.BugUpdate{Status: "ASSIGNED", AssignedTo: firstTeamCommentor}); err != nil {
					klog.Errorf("Failed to assign bug #%v to %s", b.ID)
					continue
				}
				c.slackClient.MessageEmail(comp.Lead, fmt.Sprintf("Assigned %s bug <https://bugzilla.redhat.com/show_bug.cgi?id=%v|#%v %q> to %s due to comments.", name, b.ID, b.ID, b.Summary, firstTeamCommentor))
			*/
		}
	}

	return errutil.NewAggregate(errors)
}