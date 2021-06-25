package firstteamcommentcontroller

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/eparis/bugzilla"
	"github.com/openshift/library-go/pkg/controller/factory"
	"github.com/openshift/library-go/pkg/operator/events"
	slackgo "github.com/slack-go/slack"
	errutil "k8s.io/apimachinery/pkg/util/errors"
	"k8s.io/klog"

	"github.com/mfojtik/bugzilla-operator/pkg/operator/config"
	"github.com/mfojtik/bugzilla-operator/pkg/operator/controller"
)

const assignBlockID = "first-team-comment-controller/accept-assignment"

type FirstTeamCommentController struct {
	controller.ControllerContext
	config config.OperatorConfig
}

func NewFirstTeamCommentController(ctx controller.ControllerContext, operatorConfig config.OperatorConfig, recorder events.Recorder) factory.Controller {
	c := &FirstTeamCommentController{ctx, operatorConfig}

	if err := ctx.SubscribeBlockAction(assignBlockID, c.assignClicked); err != nil {
		klog.Warning(err)
	}

	return factory.New().WithSync(c.sync).ResyncEvery(2*time.Hour).ToController("FirstTeamCommentController", recorder)
}

func (c *FirstTeamCommentController) sync(ctx context.Context, syncCtx factory.SyncContext) error {
	client := c.NewBugzillaClient(ctx)
	slackClient := c.SlackClient(ctx)
	var errors []error

	for name, comp := range c.config.Components {
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
		klog.Infof("%d NEW bugs found assigned to lead %s in component %s: %s", len(leadAssignedBugs), comp.Lead, name, strings.Join(toStringList(toIDList(leadAssignedBugs)), " "))

	nextBug:
		for _, b := range leadAssignedBugs {
			comments, err := client.GetCachedBugComments(b.ID, b.LastChangeTime)
			if err != nil {
				syncCtx.Recorder().Warningf("GetCachedBugComments", err.Error())
				continue
			}

			var firstTeamCommentor string
			onlyOneTeamCommentor := true
			for _, c := range comments {
				commentor := c.Creator
				if !strings.ContainsRune(commentor, '@') {
					commentor = commentor + "@redhat.com"
				}
				if strings.Contains(c.Text, "LifecycleStale") {
					continue
				}
				if commentor == comp.Lead {
					continue nextBug
				}
				if nonLeads.Has(commentor) && firstTeamCommentor == "" && b.Creator != commentor {
					firstTeamCommentor = commentor
				} else if nonLeads.Has(commentor) && commentor != firstTeamCommentor {
					onlyOneTeamCommentor = false
				}
			}

			if firstTeamCommentor == "" || !onlyOneTeamCommentor {
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

			klog.Infof("%s commented on #%v, but lead %s hasn't", firstTeamCommentor, b.ID, comp.Lead)

			// TODO: remove to enable for everybody
			if comp.Lead != "sttts@redhat.com" {
				return nil
			}

			value, _ := json.Marshal(AssignValue{b.ID, comp.Lead, firstTeamCommentor})
			slackClient.PostMessageEmail(comp.Lead,
				slackgo.MsgOptionBlocks(
					slackgo.NewSectionBlock(slackgo.NewTextBlockObject("mrkdwn", fmt.Sprintf("%s commented on https://bugzilla.redhat.com/show_bug.cgi?id=%v", firstTeamCommentor, b.ID), false, false), nil, nil),
					slackgo.NewActionBlock(assignBlockID,
						slackgo.NewButtonBlockElement("btn", string(value), slackgo.NewTextBlockObject("plain_text", "Assign :bugzilla:", true, false)).WithStyle(slackgo.StylePrimary),
					),
				),
			)
		}
	}

	return errutil.NewAggregate(errors)
}

type AssignValue struct {
	ID       int    `json:"id"`
	Lead     string `json:"lead"`
	AssignTo string `json:"assignTo"`
}

func (c *FirstTeamCommentController) assignClicked(ctx context.Context, message *slackgo.Container, user *slackgo.User, action *slackgo.BlockAction) {
	var value AssignValue
	if err := json.Unmarshal([]byte(action.Value), &value); err != nil {
		klog.Warningf("cannot unmarshal value %q: %v", action.Value, err)
		return
	}

	client := c.NewBugzillaClient(ctx)
	slackClient := c.SlackClient(ctx)

	if err := client.UpdateBug(value.ID, bugzilla.BugUpdate{Status: "ASSIGNED", AssignedTo: value.AssignTo}); err != nil {
		slackClient.MessageEmail(value.Lead, fmt.Sprintf("Failed to assign https://bugzilla.redhat.com/show_bug.cgi?id=%v to %s: %v", value.ID, value.AssignTo, err))
		klog.Errorf("Failed to assign bug #%v to %s", value.ID, value.AssignTo)
		return
	}

	slackClient.MessageEmail(value.Lead, fmt.Sprintf("Assigned %s bug https://bugzilla.redhat.com/show_bug.cgi?id=%v.", value.AssignTo, value.ID))
}

func toIDList(bugs []*bugzilla.Bug) []int {
	ret := make([]int, 0, len(bugs))
	for _, b := range bugs {
		ret = append(ret, b.ID)
	}
	return ret
}

func toStringList(ids []int) []string {
	ret := make([]string, 0, len(ids))
	for _, id := range ids {
		ret = append(ret, strconv.Itoa(id))
	}
	return ret
}
