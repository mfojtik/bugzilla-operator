package main

import (
	"fmt"
	"log"
	"os"
	"time"

	"github.com/mfojtik/bugzilla"
	errutil "k8s.io/apimachinery/pkg/util/errors"
)

func getFromEnvOrDie(varName string) string {
	if v := os.Getenv(varName); len(v) > 0 {
		return v
	}
	panic(fmt.Sprintf("Required %q environment variable is not set", varName))
}

func trunc(in string) string {
	if len(in) >= 120 {
		return in[0:120]
	}
	return in
}

var commentBody = `
This bug hasn't had any activity in the last 30 days. Maybe the problem got resolved, was a duplicate of something else, or became less pressing for some reason - or maybe it's still relevant but just hasn't been looked at yet.

As such, we're marking this bug as "LifecycleStale" and decreasing the severity. 

If you have further information on the current state of the bug, please update it, otherwise this bug will be closed in about 7 days. The information can be, for example, that the problem still occurs, that you still want the feature, that more information is needed, or that the bug is (for whatever reason) no longer relevant.
`

func sync(client bugzilla.Client) error {
	staleBugs, err := client.BugList("openshift-group-b-stale", "290313")
	if err != nil {
		return err
	}

	errors := []error{}
	for _, bug := range staleBugs {
		log.Printf("Stale Bug %d: %s", bug.ID, trunc(bug.Summary))
		if err := client.UpdateBug(bug.ID, bugzilla.BugUpdate{
			DevWhiteboard: "LifecycleStale",
			NeedInfo:      "1",
			NeedInfoRole:  "reporter",
			Comment: &bugzilla.BugComment{
				Body:     commentBody,
				Private:  false,
				Markdown: false,
			},
		}); err != nil {
			errors = append(errors, err)
		}
	}

	if len(errors) == 0 {
		return nil
	}

	return errutil.NewAggregate(errors)
}

func main() {
	client := bugzilla.NewClient(func() []byte {
		return []byte(getFromEnvOrDie("BUGZILLA_APIKEY"))
	}, "https://bugzilla.redhat.com").WithCGIClient(getFromEnvOrDie("BUGZILLA_USERNAME"), getFromEnvOrDie("BUGZILLA_PASSWORD"))

	for {
		if err := sync(client); err != nil {
			log.Printf("ERROR: %w", err)
		}
		time.Sleep(1 * time.Hour)
	}
}
