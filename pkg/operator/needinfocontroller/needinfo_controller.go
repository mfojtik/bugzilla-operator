package needinfocontroller

import (
	"context"
	"fmt"
	"net/url"
	"strings"
	"time"

	"github.com/eparis/bugzilla"
	"github.com/openshift/library-go/pkg/controller/factory"
	"github.com/openshift/library-go/pkg/operator/events"
	errorutil "k8s.io/apimachinery/pkg/util/errors"
	"k8s.io/klog"

	"github.com/mfojtik/bugzilla-operator/pkg/cache"
	"github.com/mfojtik/bugzilla-operator/pkg/operator/bugutil"
	"github.com/mfojtik/bugzilla-operator/pkg/operator/config"
	"github.com/mfojtik/bugzilla-operator/pkg/operator/controller"
	slack "github.com/mfojtik/bugzilla-operator/pkg/slack"
)

type NeedInfoController struct {
	controller.ControllerContext
	config config.OperatorConfig
}

func NewNeedInfoController(ctx controller.ControllerContext, operatorConfig config.OperatorConfig, recorder events.Recorder) factory.Controller {
	c := &NeedInfoController{ctx, operatorConfig}
	return factory.New().WithSync(c.sync).ResyncEvery(30*time.Minute).ToController("NeedInfoController", recorder)
}

func (c *NeedInfoController) sync(ctx context.Context, syncCtx factory.SyncContext) (err error) {
	client := c.NewBugzillaClient(ctx)
	slackClient := c.SlackClient(ctx)

	since := time.Now().Add(-time.Hour * 24 * 7)
	sinceKey := "needinfo-controller.since"
	if s, err := c.GetPersistentValue(ctx, sinceKey); err != nil {
		return err
	} else if s != "" {
		if t, err := time.Parse(time.RFC3339, s); err != nil {
			klog.Warningf("Cannot parse time %q for key %s: %v", s, sinceKey, err)
		} else {
			since = t
		}
	}

	newSince, err := Report(ctx, client, slackClient, syncCtx.Recorder(), c.config.Components.List(), since)
	if err == nil && !newSince.IsZero() {
		if persistErr := c.SetPersistentValue(ctx, sinceKey, newSince.Format(time.RFC3339)); persistErr != nil {
			klog.Warningf("Cannot persist key %s: %v", sinceKey, persistErr)
		}
	}

	return err
}

func Report(ctx context.Context, client cache.BugzillaClient, slackClient slack.ChannelClient, recorder events.Recorder, components []string, since time.Time) (time.Time, error) {
	if since.IsZero() {
		since = time.Now().Add(-time.Hour * 24 * 7)
	}

	bugs, err := getNewBugs(client, components, since)
	if err != nil {
		recorder.Warningf("BuglistFailed", err.Error())
		return time.Time{}, err
	}

	var errs []error
	lastSeenChange := since
nextBug:
	for _, b := range bugs {
		lastChange, err := time.Parse(time.RFC3339, b.LastChangeTime)
		if err != nil {
			klog.Warningf("Cannot parse last-change-time %q of #%d: %v", b.LastChangeTime, b.ID, err)
		} else if lastChange.After(lastSeenChange) {
			lastSeenChange = lastChange
		}

		// ignore changes at exactly the edge because we can only search for >=
		if !lastChange.After(since) {
			continue
		}

		for _, f := range b.Flags {
			if f.Name != "needinfo" || f.Status != "?" {
				continue
			}

			// ignore needinfo? flag for other people
			if f.Requestee != b.AssignedTo {
				continue
			}

			if flagDate, err := time.Parse(time.RFC3339, f.ModificationDate); err != nil {
				klog.Warningf("Cannot parse assignee needinfo? modification time %q of #%d: %v", f.ModificationDate, b.ID, err)
				continue
			} else if flagDate.After(since) {
				slackClient.MessageEmail(b.AssignedTo, fmt.Sprintf(":parrotdad: %s has set `needinfo?` *on you* on: %s", f.Setter, bugutil.FormatBugMessage(*b)))
				continue nextBug
			}
		}

		history, err := client.GetCachedBugHistory(b.ID, b.LastChangeTime)
		if err != nil {
			errs = append(errs, err)
			continue
		}

		somebodyElsesNeedInfoRemoved := false
		var by string
		for _, h := range history {
			if when, err := time.Parse(time.RFC3339, h.When); err != nil {
				continue
			} else if when.Before(since) {
				continue
			}

			if h.Who == b.AssignedTo {
				continue
			}

			for _, c := range h.Changes {
				if c.FieldName != "flagtypes.name" {
					continue
				}

				if strings.Contains(strings.ReplaceAll(c.Removed, fmt.Sprintf("needinfo?(%s)", b.AssignedTo), ""), "needinfo?(") {
					somebodyElsesNeedInfoRemoved = true
					by = h.Who
					break // keep going with later history to get last person proving info
				}
			}
		}

		if somebodyElsesNeedInfoRemoved {
			slackClient.MessageEmail(b.AssignedTo, fmt.Sprintf(":parrotdad: %s *provided requested info* on: %s", by, bugutil.FormatBugMessage(*b)))
		}
	}

	return lastSeenChange, errorutil.NewAggregate(errs)
}

func getNewBugs(client cache.BugzillaClient, components []string, changedAfter time.Time) ([]*bugzilla.Bug, error) {
	return client.Search(bugzilla.Query{
		Classification: []string{"Red Hat"},
		Product:        []string{"OpenShift Container Platform"},
		Status:         []string{"NEW", "ASSIGNED", "POST", "ON_DEV"},
		Component:      components,
		Raw:            fmt.Sprintf("last_change_time=%s", url.QueryEscape(changedAfter.Format("2006-01-02T15:04:05Z"))),
		IncludeFields: []string{
			"id",
			"creation_time",
			"last_change_time",
			"assigned_to",
			"reporter",
			"severity",
			"priority",
			"flags",
			"summary",
		},
	})
}
