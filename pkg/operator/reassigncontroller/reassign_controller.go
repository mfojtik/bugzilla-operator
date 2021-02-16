package reassigncontroller

import (
	"context"
	"time"

	"k8s.io/klog"

	"github.com/eparis/bugzilla"
	"github.com/openshift/library-go/pkg/controller/factory"
	"github.com/openshift/library-go/pkg/operator/events"

	"github.com/mfojtik/bugzilla-operator/pkg/cache"
	"github.com/mfojtik/bugzilla-operator/pkg/operator/config"
	"github.com/mfojtik/bugzilla-operator/pkg/operator/controller"
)

type ReassignController struct {
	context controller.ControllerContext
	config  *config.OperatorConfig
}

func NewReassignController(ctx controller.ControllerContext, operatorConfig config.OperatorConfig, recorder events.Recorder) factory.Controller {
	c := &ReassignController{context: ctx, config: &operatorConfig}
	return factory.New().WithSync(c.sync).ResyncEvery(1*time.Hour).ToController("ReassignController", recorder)
}

func (c *ReassignController) sync(ctx context.Context, syncCtx factory.SyncContext) (err error) {
	client := c.context.NewBugzillaClient(ctx)
	assigneeNotifiedBugs, err := getAssigneeNotifiedBugsList(client, c.config)
	if err != nil {
		syncCtx.Recorder().Warningf("BuglistFailed", err.Error())
		return err
	}

	for _, bug := range assigneeNotifiedBugs {
		history, err := client.GetCachedBugHistory(bug.ID, bug.LastChangeTime)
		if err != nil {
			syncCtx.Recorder().Warningf("GetBugFailed", err.Error())
			continue
		}
		var (
			whenNotified         time.Time
			whenComponentChanged *time.Time
		)
		for _, h := range history {
			changedAt, err := time.Parse(time.RFC3339, h.When)
			if err != nil {
				klog.Errorf("unable to parse change time %q for bug %+v: %v", h.When, bug, err)
				continue
			}
			for _, change := range h.Changes {
				switch change.FieldName {
				case "cf_devel_whiteboard":
					whenNotified = changedAt
				case "component":
					whenComponentChanged = &changedAt
				}
			}
		}

		if whenComponentChanged != nil && whenComponentChanged.After(whenNotified) {
			if err := client.UpdateBug(bug.ID, bugzilla.BugUpdate{
				DevWhiteboard: "",
				MinorUpdate:   true,
			}); err != nil {
				syncCtx.Recorder().Warningf("ResetNotificationFailed", "Failed to updated bug #%d: %v", bug.ID, err)
				continue
			}
			syncCtx.Recorder().Eventf("BugReassigned", "Cleared AssigneeNotified for bug #%d because component changed", bug.ID)
		}
	}
	return nil
}

func getAssigneeNotifiedBugsList(client cache.BugzillaClient, config *config.OperatorConfig) ([]*bugzilla.Bug, error) {
	return client.Search(bugzilla.Query{
		Classification: []string{"Red Hat"},
		Product:        []string{"OpenShift Container Platform"},
		Status:         []string{"NEW", "ASSIGNED"},
		Component:      config.Components.List(),
		Advanced: []bugzilla.AdvancedQuery{
			{
				Field: "cf_devel_whiteboard",
				Op:    "substring",
				Value: "AssigneeNotified",
			},
		},
		IncludeFields: []string{
			"id",
			"assigned_to",
			"keywords",
			"status",
			"component",
			"resolution",
			"summary",
			"severity",
			"priority",
			"target_release",
			"whiteboard",
		},
	})
}
