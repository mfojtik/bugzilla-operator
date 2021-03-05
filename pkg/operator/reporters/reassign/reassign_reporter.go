package reassign

import (
	"context"
	"encoding/json"
	"fmt"
	"reflect"
	"sort"
	"strings"
	"time"

	"github.com/eparis/bugzilla"
	"k8s.io/apimachinery/pkg/util/sets"

	"github.com/openshift/library-go/pkg/controller/factory"
	"github.com/openshift/library-go/pkg/operator/events"

	"github.com/mfojtik/bugzilla-operator/pkg/cache"
	"github.com/mfojtik/bugzilla-operator/pkg/operator/bugutil"
	"github.com/mfojtik/bugzilla-operator/pkg/operator/config"
	"github.com/mfojtik/bugzilla-operator/pkg/operator/controller"
)

// ReassignReporter lists all OpenShift bugs which has a change recorded in last 7 days. If that change was component change it persist the incoming and outgoing (target)
// component as "component change". It reports then the top incoming and outgoing component for last 7 days.
type ReassignReporter struct {
	controller.ControllerContext
	config config.OperatorConfig

	// for unit testing
	writeReportFn func(ctx context.Context, bugs []bug) error
	readReportFn  func(ctx context.Context) ([]bug, error)
	getBugHistory func(int, string) ([]bugzilla.History, error)
}

type componentChange struct {
	From string    `json:"from"`
	To   string    `json:"to"`
	When time.Time `json:"when"`
}

type bug struct {
	ID                  int               `json:"id"`
	ComponentChanges    []componentChange `json:"component_changes"`
	LastComponentChange time.Time         `json:"last_component_change"`
}

type componentCounter struct {
	counts []componentCount
}

func (c *componentCounter) byCount() []componentCount {
	sortedCounter := c.counts
	sort.Slice(c.counts, func(i, j int) bool {
		return sortedCounter[i].count >= sortedCounter[j].count
	})
	return sortedCounter
}

func (c *componentCounter) Inc(name string) {
	for i := range c.counts {
		if c.counts[i].name == name {
			c.counts[i].count += 1
			return
		}
	}
	c.counts = append(c.counts, componentCount{
		name:  name,
		count: 1,
	})
}

type componentCount struct {
	name  string
	count int
}

func Report(ctx context.Context, controllerCtx controller.ControllerContext, recorder events.Recorder, config *config.OperatorConfig) (string, error) {
	c := &ReassignReporter{ControllerContext: controllerCtx}
	currentBugs, err := c.getReport(ctx)
	if err != nil {
		return "", err
	}

	topToComponents := &componentCounter{}
	topFromComponents := &componentCounter{}

	for _, b := range currentBugs {
		// only report component changes that happened in last week
		if !b.LastComponentChange.After(time.Now().Add(-7 * 24 * time.Hour)) {
			continue
		}
		for _, c := range b.ComponentChanges {
			topFromComponents.Inc(c.From)
			topToComponents.Inc(c.To)
		}
	}

	components := sets.NewString(config.Components.List()...)
	result := []string{
		"*Components we received bugs from last 7 days*:",
	}

	for _, c := range topFromComponents.byCount() {
		if components.Has(c.name) {
			continue
		}
		result = append(result, fmt.Sprintf("* %s (%d bugs)", c.name, c.count))
	}

	result = append(result, "*Components we moved bugs for last 7 days:*")
	for _, c := range topToComponents.byCount() {
		result = append(result, fmt.Sprintf("* %s (%d bugs)", c.name, c.count))
	}

	return strings.Join(result, "\n"), nil
}

func NewReassignReporter(controllerCtx controller.ControllerContext, schedule []string, operatorConfig config.OperatorConfig, recorder events.Recorder) factory.Controller {
	c := &ReassignReporter{
		ControllerContext: controllerCtx,
		config:            operatorConfig,
	}

	c.readReportFn = c.getReport
	c.writeReportFn = c.writeReport
	c.getBugHistory = c.NewBugzillaClient(context.TODO()).GetCachedBugHistory

	return factory.New().WithSync(c.sync).ResyncSchedule(schedule...).ToController("ReassignReporter", recorder)
}

func (c *ReassignReporter) sync(ctx context.Context, syncContext factory.SyncContext) error {
	client := c.NewBugzillaClient(ctx)

	bugs, err := getReassignedBugs(client, &c.config)
	if err != nil {
		syncContext.Recorder().Warningf("FetchBugs", "Unable to fetch reassigned bugs: %v", err)
		return err
	}

	bugsWithTransitions, err := c.readReportFn(ctx)
	if err != nil {
		return err
	}

	for _, b := range bugs {
		history, err := c.getBugHistory(b.ID, b.LastChangeTime)
		if err != nil {
			syncContext.Recorder().Warningf("GetBugHistory", "Failed to get bug %d history: %v", b.ID, err)
		}
		transitions := []componentChange{}
		var lastComponentChange time.Time

		for _, h := range history {
			for _, change := range h.Changes {
				if change.FieldName != "component" {
					continue
				}

				when := bugutil.ParseChangeWhenString(h.When)
				if when.After(lastComponentChange) {
					lastComponentChange = when
				}

				transitions = append(transitions, componentChange{
					When: when,
					From: change.Removed,
					To:   change.Added,
				})
			}

		}

		// check whether the transition of the bug include component (from or to) we watch (configured in config)
		allTransitions := []componentChange{}
		allTransitions = append(allTransitions, transitions...)
		allTransitions = append(allTransitions, getCachedBugTransitions(bugsWithTransitions, b.ID)...)
		if !c.matchConfiguredComponent(allTransitions) {
			continue
		}

		pos, hasChange := hasNewTransitions(bugsWithTransitions, b.ID, transitions)
		if !hasChange {
			continue
		}

		if pos != -1 { // existing bug
			bugsWithTransitions[pos] = bug{
				ID:                  b.ID,
				ComponentChanges:    transitions,
				LastComponentChange: lastComponentChange,
			}
		} else { // new bug
			bugsWithTransitions = append(bugsWithTransitions, bug{
				ID:                  b.ID,
				ComponentChanges:    transitions,
				LastComponentChange: lastComponentChange,
			})
		}
	}

	return c.writeReportFn(ctx, bugsWithTransitions)
}

func getCachedBugTransitions(bugs []bug, id int) []componentChange {
	for i := range bugs {
		if bugs[i].ID == id {
			return bugs[i].ComponentChanges
		}
	}
	return []componentChange{}
}

func getCachedBugPos(bugs []bug, id int) int {
	for i := range bugs {
		if id == bugs[i].ID {
			return i
		}
	}
	return -1
}

// hasNewTransitions checks whether a bug has new component changes or not.
// if the bug is not persisted in storage, this returns true.
// it returns index in storage where the bug is persisted.
func hasNewTransitions(bugs []bug, id int, newChanges []componentChange) (int, bool) {
	oldChanges := getCachedBugTransitions(bugs, id)
	// this is new bug, we should add it to cache
	if len(oldChanges) == 0 {
		return -1, true
	}
	sort.Slice(newChanges, func(i, j int) bool {
		return newChanges[i].When.After(newChanges[j].When)
	})
	sort.Slice(oldChanges, func(i, j int) bool {
		return oldChanges[i].When.After(oldChanges[j].When)
	})
	// no changes
	if reflect.DeepEqual(oldChanges, newChanges) {
		return -1, false
	}
	// we track this bug and it has new changes
	return getCachedBugPos(bugs, id), true
}

// writeReport persist the current bug component transitions into permanent storage (config map) in JSON
func (c *ReassignReporter) writeReport(ctx context.Context, bugs []bug) error {
	bytes, err := json.Marshal(&bugs)
	if err != nil {
		return err
	}
	return c.SetPersistentValue(ctx, "transitions", string(bytes))
}

// getReport gets the current bug component transitions from permanent storage (config map) in JSON
func (c *ReassignReporter) getReport(ctx context.Context) ([]bug, error) {
	bugJSON, err := c.GetPersistentValue(ctx, "transitions")
	if err != nil {
		return nil, err
	}
	if len(bugJSON) == 0 {
		return []bug{}, nil
	}
	var bugs []bug
	if err := json.Unmarshal([]byte(bugJSON), &bugs); err != nil {
		return nil, err
	}
	return bugs, nil
}

// matchConfiguredComponent check whether the transition include component of interest (from config)
func (c *ReassignReporter) matchConfiguredComponent(transitions []componentChange) bool {
	components := sets.NewString(c.config.Components.List()...)
	for _, t := range transitions {
		if components.Has(t.From) || components.Has(t.To) {
			return true
		}
	}
	return false
}

func getReassignedBugs(client cache.BugzillaClient, config *config.OperatorConfig) ([]*bugzilla.Bug, error) {
	return client.Search(bugzilla.Query{
		Classification: []string{"Red Hat"},
		Product:        []string{"OpenShift Container Platform"},
		Status:         []string{"NEW", "ASSIGNED", "POST"},
		Component:      config.Components.List(),
		Advanced: []bugzilla.AdvancedQuery{
			{
				// get all bugs that changed their component in the last week
				Field: "component",
				Op:    "changedafter",
				Value: "-1w",
			},
		},
		IncludeFields: []string{
			"id",
		},
	})
}
