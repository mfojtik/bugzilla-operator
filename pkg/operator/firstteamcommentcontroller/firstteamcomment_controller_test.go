package firstteamcommentcontroller

import (
	"os"
	"testing"

	"github.com/eparis/bugzilla"
	"github.com/openshift/library-go/pkg/controller/factory"
	"github.com/openshift/library-go/pkg/operator/events"

	"github.com/mfojtik/bugzilla-operator/pkg/cache"
	"github.com/mfojtik/bugzilla-operator/pkg/operator/config"
)

func TestNewFirstTeamCommentController(t *testing.T) {
	// SKIPPED
	// remove to run locally
	return

	cache.Open("/tmp/bolt")
	c := &FirstTeamCommentController{
		config: config.OperatorConfig{
			Groups: map[string]config.Group{
				"admins":   {"mfojtik@redhat.com", "sttts@redhat.com"},
				"leads":    {"deads@redhat.com", "sbatsche@redhat.com", "maszulik@redhat.com", "mfojtik@redhat.com", "sttts@redhat.com"},
				"api-auth": {"sttts@redhat.com", "deads@redhat.com", "lszaszki@redhat.com", "slaznick@redhat.com", "akashem@redhat.com", "lusanche@redhat.com", "vareti@redhat.com", "mnewby@redhat.com"},
			},
			Components: map[string]config.Component{
				"kube-apiserver": {
					Lead:                          "sttts@redhat.com",
					Developers:                    []string{"group:api-auth"},
					AssignFirstDeveloperCommentor: true,
				},
				"api-auth": {
					Lead:                          "sttts@redhat.com",
					Developers:                    []string{"group:api-auth"},
					AssignFirstDeveloperCommentor: true,
				},
				"openshift-apiserver": {
					Lead:                          "sttts@redhat.com",
					Developers:                    []string{"group:api-auth"},
					AssignFirstDeveloperCommentor: true,
				},
			},
		},
		newBugzillaClient: func() cache.BugzillaClient {
			return cache.NewCachedBugzillaClient(bugzilla.NewClient(func() []byte {
				return []byte(os.Getenv("BUGZILLA_TOKEN"))
			}, "https://bugzilla.redhat.com"))
		},
	}
	c.sync(nil, factory.NewSyncContext("foo", events.NewLoggingEventRecorder("TestNewFirstTeamCommentController")))
}
