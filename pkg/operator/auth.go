package operator

import (
	"strings"

	"k8s.io/apimachinery/pkg/util/sets"

	"github.com/mfojtik/bugzilla-operator/pkg/operator/config"
	"github.com/mfojtik/bugzilla-operator/pkg/slacker"
)

func auth(cfg config.OperatorConfig, handler func(req slacker.Request, w slacker.ResponseWriter), restrictedTo ...string) func(req slacker.Request, w slacker.ResponseWriter) {
	users := sets.String{}
	for _, x := range restrictedTo {
		users, _ = expandGroup(cfg.Groups, x, users, nil)
	}

	return func(req slacker.Request, w slacker.ResponseWriter) {
		if !users.Has(req.Event().Username) {
			w.Reply("Permission denied")
		}
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
