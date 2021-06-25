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

	"github.com/mfojtik/bugzilla-operator/pkg/operator/bugutil"
	"github.com/mfojtik/bugzilla-operator/pkg/operator/config"
	"github.com/mfojtik/bugzilla-operator/pkg/operator/controller"
)

const assignBlockID = "first-team-comment-controller/accept-assignment"

type FirstTeamCommentController struct {
	controller.ControllerContext
	config        config.OperatorConfig
	slackGoClient *slackgo.Client
}

func NewFirstTeamCommentController(ctx controller.ControllerContext, operatorConfig config.OperatorConfig, slackGoClient *slackgo.Client, recorder events.Recorder) factory.Controller {
	c := &FirstTeamCommentController{
		ctx,
		operatorConfig,
		slackGoClient,
	}

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

		since := time.Now().Add(-time.Hour * 24 * 365)
		sinceKey := "first-team-comment-controller.since-" + name
		if s, err := c.GetPersistentValue(ctx, sinceKey); err != nil {
			return err
		} else if s != "" {
			if t, err := time.Parse(time.RFC3339, s); err != nil {
				klog.Warningf("Cannot parse time %q for key %s: %v", s, sinceKey, err)
			} else {
				since = t
			}
		}
		newSince := time.Now()

		query := fmt.Sprintf(
			"email1=%s&email2=%s&emailassigned_to2=1&emaillongdesc1=1&emaillongdesc3=1&emailtype1=regexp&emailtype2=equals&last_change_time=%s",
			url.QueryEscape(strings.Join(nonLeads.List(), "|")),
			url.QueryEscape(comp.Lead),
			url.QueryEscape(since.Format("2006-01-02T15:04:05Z")),
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
					createdAt, err := time.Parse("2006-01-02T15:04:05Z", c.CreationTime)
					if err == nil && createdAt.Before(since) {
						// we must have seen this before and notified
						continue nextBug
					}

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

			value, _ := json.Marshal(AssignValue{b.ID, comp.Lead, firstTeamCommentor})
			text := fmt.Sprintf("%s commented as first team member:\n\n%s", firstTeamCommentor, bugutil.FormatBugMessage(*b))
			slackClient.PostMessageEmail(comp.Lead,
				slackgo.MsgOptionBlocks(
					slackgo.NewSectionBlock(slackgo.NewTextBlockObject("mrkdwn", text, false, false), nil, nil),
					slackgo.NewActionBlock(assignBlockID,
						slackgo.NewButtonBlockElement("btn", string(value), slackgo.NewTextBlockObject("plain_text", "Auto-Assign", true, false)).WithStyle(slackgo.StylePrimary),
					),
				),
			)
		}

		if persistErr := c.SetPersistentValue(ctx, sinceKey, newSince.Format(time.RFC3339)); persistErr != nil {
			klog.Warningf("Cannot persist key %s: %v", sinceKey, persistErr)
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

	// we only have 3s to respond to Slack, but BZ might take longer. Do the work in a go routine
	client := c.NewBugzillaClient(context.Background())
	slackClient := c.SlackClient(context.Background())
	go func() {
		b, _, err := client.GetCachedBug(value.ID, "")
		if err != nil {
			slackClient.MessageEmail(value.Lead, fmt.Sprintf("Failed to get https://bugzilla.redhat.com/show_bug.cgi?id=%v: %v", value.ID, err))
			klog.Errorf("Failed to get bug #%v: %v", value.ID, err)
			return
		}
		if b.Status != "NEW" {
			slackClient.MessageEmail(value.Lead, fmt.Sprintf("Bug https://bugzilla.redhat.com/show_bug.cgi?id=%v has been moved already to %s", value.ID, b.Status))
			return
		}
		if b.AssignedTo != "" && b.AssignedTo != value.Lead {
			slackClient.MessageEmail(value.Lead, fmt.Sprintf("Bug https://bugzilla.redhat.com/show_bug.cgi?id=%v has already been assigned to %s", value.ID, value.Lead))
			return
		}

		if err := client.UpdateBug(value.ID, bugzilla.BugUpdate{Status: "ASSIGNED", AssignedTo: value.AssignTo}); err != nil {
			slackClient.MessageEmail(value.Lead, fmt.Sprintf("Failed to assign https://bugzilla.redhat.com/show_bug.cgi?id=%v to %s: %v", value.ID, value.AssignTo, err))
			klog.Errorf("Failed to assign bug #%v to %s: %v", value.ID, value.AssignTo, err)
			return
		}

		text := fmt.Sprintf("%s – assigned to %s", bugutil.FormatBugMessage(*b), value.AssignTo)
		klog.Infof("Updating message to: %v", text)
		if _, _, _, err := c.slackGoClient.UpdateMessage(
			message.ChannelID,
			message.MessageTs,
			slackgo.MsgOptionText(text, false),
		); err != nil {
			slackClient.MessageEmail(value.Lead, fmt.Sprintf("Assigned %s bug https://bugzilla.redhat.com/show_bug.cgi?id=%v.", value.AssignTo, value.ID))
			klog.Errorf("Failed to update message: %v", err)
		}

	}()
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
