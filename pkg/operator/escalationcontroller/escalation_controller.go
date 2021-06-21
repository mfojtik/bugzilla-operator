package escalationcontroller

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/eparis/bugzilla"
	"github.com/openshift/library-go/pkg/controller/factory"
	"github.com/openshift/library-go/pkg/operator/events"
	errorutil "k8s.io/apimachinery/pkg/util/errors"

	"github.com/mfojtik/bugzilla-operator/pkg/cache"
	"github.com/mfojtik/bugzilla-operator/pkg/operator/bugutil"
	"github.com/mfojtik/bugzilla-operator/pkg/operator/config"
	"github.com/mfojtik/bugzilla-operator/pkg/operator/controller"
	"github.com/mfojtik/bugzilla-operator/pkg/slack"
)

const EmergencyRequestMessage = `** WARNING **

This BZ claims that this bug is of urgent severity and priority. Note that urgent priority means that you just declared emergency within engineering. 
Engineers are asked to stop whatever they are doing, including putting important release work on hold, potentially risking the release of OCP while working on this case.

Be prepared to have a good justification ready and your own and engineering management are aware and has approved this. Urgent bugs are very expensive and have maximal management visibility.

NOTE: This bug was assigned to engineering manager with severity reset to *unspecified* until the emergency is vetted and confirmed. Please do not manually override the severity.
`

type escalation struct {
	BugID int `yaml:"bugID"`
}

type EscalationController struct {
	controller.ControllerContext
	config config.OperatorConfig
}

func NewEscalationController(ctx controller.ControllerContext, operatorConfig config.OperatorConfig, recorder events.Recorder) factory.Controller {
	c := &EscalationController{ControllerContext: ctx, config: operatorConfig}
	return factory.New().WithSync(c.sync).ResyncEvery(30*time.Minute).ToController("EscalationController", recorder)
}

func (c *EscalationController) sync(ctx context.Context, context factory.SyncContext) error {
	client := c.NewBugzillaClient(ctx)
	slackClient := c.SlackClient(ctx)
	var errors []error

	urgentBugs, err := getUrgentBugs(client, c.config.Components.List())
	if err != nil {
		context.Recorder().Warningf("GetUrgentBugsFailed", "Failed to fetch urgent bugs: %v", err)
		return err
	}

	update := map[int]bugzilla.BugUpdate{}
	summaryForStatusChan := []bugzilla.Bug{}

	for _, b := range urgentBugs {
		// This urgent/urgent bug has been confirmed as emergency, skip it as we are done with it.
		if strings.Contains(b.Whiteboard, "EmergencyConfirmed") {
			continue
		}
		// This urgent/urgent bug has not requested emergency vetting, notify managers and drop summary into status channel.
		// Managers will get 1 notification, but we will keep posting message to status channel until there is an action.
		if !c.isTriageRequested(ctx, b) {
			errors = append(errors, c.requestUrgentTriage(b, slackClient, context.Recorder()))
			summaryForStatusChan = append(summaryForStatusChan, *b)
		}
		// This urgent/urgent bug has not requested emergency vetting or somebody removed the whiteboard keyboard and reset
		// the severity back to urgent. This will stomp that change.
		// The bug will be also temporarily assigned to manager of a team owning this component.
		if !strings.Contains(b.Whiteboard, "EmergencyRequest") {
			// This urgent/urgent bug is
			update[b.ID] = bugzilla.BugUpdate{
				Whiteboard: withKeyword(b.Whiteboard, "EmergencyRequest"),
				Comment: &bugzilla.BugComment{
					Body: EmergencyRequestMessage,
				},
				AssignedTo: c.config.Components.ManagerFor(b.Component[0], b.AssignedTo),
				Priority:   "unspecified",
				Severity:   "unspecified",
			}
		}
	}

	for id, u := range update {
		if err := client.UpdateBug(id, u); err != nil {
			errors = append(errors, err)
		}
	}

	if err := c.summaryForStatusChan(summaryForStatusChan, slackClient); err != nil {
		errors = append(errors, err)
	}

	if len(errors) > 0 {
		return errorutil.NewAggregate(errors)
	}
	return nil
}

// isTriageRequested determine whether this bug has vetting/triage already requested from management.
// It returns true if management received notification via Slack about this emergency.
func (c *EscalationController) isTriageRequested(ctx context.Context, b *bugzilla.Bug) bool {
	bytes, err := c.GetPersistentValue(ctx, "escalations")
	if err != nil {
		return false
	}
	var escalations []escalation
	if err := json.Unmarshal([]byte(bytes), &escalations); err != nil {
		return false
	}
	for _, e := range escalations {
		if b.ID == e.BugID {
			return true
		}
	}
	return false
}

// markUrgentTriageNotified marks a bug when the notification to management was successfully sent.
func (c *EscalationController) markUrgentTriageNotified(ctx context.Context, b *bugzilla.Bug) error {
	bytes, err := c.GetPersistentValue(ctx, "escalations")
	if err != nil {
		return err
	}
	var escalations []escalation
	if len(bytes) != 0 {
		if err := json.Unmarshal([]byte(bytes), &escalations); err != nil {
			return err
		}
	}
	escalations = append(escalations, escalation{BugID: b.ID})
	escalationsBytes, err := json.Marshal(escalations)
	if err != nil {
		return err
	}
	return c.SetPersistentValue(ctx, "escalations", string(escalationsBytes))
}

// requestUrgentTriage will contact component product manager AND engineering manager via Slack to request immediate vetting on this emergency.
func (c *EscalationController) requestUrgentTriage(b *bugzilla.Bug, slackClient slack.ChannelClient, recorder events.Recorder) error {
	recipients := []string{
		c.config.Components.ManagerFor(b.Component[0], b.AssignedTo),
		c.config.Components.ProductManagerFor(b.Component[0], b.AssignedTo),
	}
	var errors []error
	for _, recipient := range recipients {
		if len(recipient) == 0 {
			recorder.Warningf("RecipientMissing", "Component %s is missing product manager or manager", b.Component[0])
			continue
		}
		err := slackClient.MessageEmail(recipient, fmt.Sprintf(":alert-siren: *The following bug require immediate attention and triage*: %s\n"+
			"> _Please contact engineering if necessary and add the *EmergencyConfirmed* keyword to Whiteboard field if this is an emergency. Otherwise decrease the priority/severity._", bugutil.GetBugURL(*b)))
		if err != nil {
			errors = append(errors, err)
		}
	}
	if len(errors) == 0 {
		if err := c.markUrgentTriageNotified(context.Background(), b); err != nil {
			recorder.Warningf("MarkUrgentError", "Failed to mark bug %s as escalation notified: %v", bugutil.GetBugURL(*b), err)
			errors = append(errors, err)
		}
	}
	return errorutil.NewAggregate(errors)
}

func (c *EscalationController) summaryForStatusChan(statusChan []bugzilla.Bug, client slack.ChannelClient) error {
	message := []string{}
	for _, b := range statusChan {
		message = append(message, bugutil.FormatBugMessage(b))
	}
	if len(message) == 0 {
		return nil
	}
	return client.MessageAdminChannel(fmt.Sprintf(":alert-siren: Requested PM and manager vetting on follow bugs:\n%s", strings.Join(message, "\n")))
}

func getUrgentBugs(client cache.BugzillaClient, components []string) ([]*bugzilla.Bug, error) {
	return client.Search(bugzilla.Query{
		Classification: []string{"Red Hat"},
		Product:        []string{"OpenShift Container Platform"},
		Status:         []string{"NEW"},
		Component:      components,
		Severity:       []string{"urgent"},
		IncludeFields: []string{
			"id",
			"assigned_to",
			"reporter",
			"severity",
			"component",
			"priority",
			"summary",
			"status",
			"status_whiteboard",
		},
	})
}

func withKeyword(wb string, kwd string) string {
	if strings.Contains(wb, kwd) {
		return wb
	}
	return strings.TrimSpace(strings.TrimSpace(wb) + " " + kwd)
}
