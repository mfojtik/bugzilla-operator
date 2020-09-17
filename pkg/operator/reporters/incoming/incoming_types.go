package incoming

import (
	"context"
	"encoding/json"
	"time"

	"github.com/mfojtik/bugzilla-operator/pkg/operator/controller"

	"github.com/eparis/bugzilla"
)

// ComponentCount is a single component name with the bug count.
type ComponentCount struct {
	Name  string `json:"name"`
	Count int    `json:"count"`
}

// SeverityCount is a single severity (urgent, high, medium, low, unspecified) with bug count.
type SeverityCount struct {
	Name  string `json:"name"`
	Count int    `json:"count"`
}

// IncomingDailyReport represents a single day report of incoming bugs broke down to components and severities for incoming bugs.
type IncomingDailyReport struct {
	Timestamp time.Time `json:"timestamp"`

	// Count daily incoming numbers for components
	Components []ComponentCount `json:"components"`

	// Count daily incoming numbers for severity
	Severities []SeverityCount `json:"severities"`
}

// IncomingReport is structure we serialize into JSON and store in config map that contain list of daily incoming bug reports.
type IncomingReport struct {
	Reports []IncomingDailyReport `json:"reports"`
}

// asJSONString serialize the incoming report to JSON string
func (r IncomingReport) asJSONString() (string, error) {
	bytes, err := json.Marshal(&r)
	if err != nil {
		return "", err
	}
	return string(bytes), nil
}

// incomingReportFromJSONString decode string that contain report in JSON
func incomingReportFromJSONString(in string) (IncomingReport, error) {
	r := IncomingReport{}
	if len(in) == 0 {
		return IncomingReport{}, nil
	}
	err := json.Unmarshal([]byte(in), &r)
	return r, err
}

// updateIncomingReport append the bugs to daily incoming bug report.
func updateIncomingReport(c controller.ControllerContext, bugs []*bugzilla.Bug) error {
	todayReport := &IncomingDailyReport{
		Timestamp: time.Now(),
	}

	componentReport := map[string]int{}
	severityReport := map[string]int{}
	for _, b := range bugs {
		componentReport[b.Component[0]] += 1
		severityReport[b.Severity] += 1
	}
	for c, count := range componentReport {
		todayReport.Components = append(todayReport.Components, ComponentCount{
			Name:  c,
			Count: count,
		})
		todayReport.Severities = append(todayReport.Severities, SeverityCount{
			Name:  c,
			Count: count,
		})
	}

	reportString, err := c.GetPersistentValue(context.TODO(), "incoming-report")
	if err != nil {
		return err
	}

	report, err := incomingReportFromJSONString(reportString)
	if err != nil {
		return err
	}

	// cap on last 30d of reports (2x a day)
	if len(report.Reports) > 62 {
		report.Reports = append(report.Reports[1:len(report.Reports)-1], *todayReport)
	}

	reportJSON, err := report.asJSONString()
	if err != nil {
		return err
	}

	return c.SetPersistentValue(context.TODO(), "incoming-report", reportJSON)
}
