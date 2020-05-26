package operator

import (
	"fmt"
	"strings"

	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/klog"

	"github.com/mfojtik/bugzilla-operator/pkg/operator/config"
	"github.com/mfojtik/bugzilla-operator/pkg/slack"
	"github.com/mfojtik/bugzilla-operator/pkg/slacker"
)

func auth(cfg config.OperatorConfig, handler func(req slacker.Request, w slacker.ResponseWriter), restrictedTo ...string) func(req slacker.Request, w slacker.ResponseWriter) {
	users := sets.String{}
	for _, x := range restrictedTo {
		users, _ = expandGroup(cfg.Groups, x, users, nil)
	}

	return func(req slacker.Request, w slacker.ResponseWriter) {
		denied := func() {
			w.Reply(fmt.Sprintf("Permission denied: User %q (%q) does not have permission to run this command", req.Event().Username, req.Event().User))
		}
		if len(req.Event().Username) == 0 || !users.Has(req.Event().Username) {
			u, err := w.Client().GetUserInfo(req.Event().User)
			if err != nil {
				denied()
				klog.Error(err)
				return
			}

			if len(u.Profile.Email) == 0 || !users.Has(slack.BugzillaToSlackEmail(u.Profile.Email)) {
				denied()
				return
			}
		}

		handler(req, w)
	}
}

func expandGroup(cfg map[string]config.Group, x string, expanded sets.String, seen sets.String) (sets.String, sets.String) {
	if strings.HasPrefix(x, "group:") {
		group := x[6:]
		if seen.Has(group) {
			return expanded, seen
		}
		if seen == nil {
			seen = sets.String{}
		}
		seen = seen.Insert(group)
		for _, y := range cfg[group] {
			expanded, seen = expandGroup(cfg, y, expanded, seen)
		}
		return expanded, seen
	}

	return expanded.Insert(x), seen
}
