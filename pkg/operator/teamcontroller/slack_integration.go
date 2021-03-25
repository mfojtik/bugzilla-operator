package teamcontroller

import (
	"fmt"
	"os"
	"strings"

	"k8s.io/klog"

	"github.com/mfojtik/bugzilla-operator/pkg/slacker"
)

func AddSlackCommands(slackerInstance *slacker.Slacker) {
	if _, err := os.Stat("/var/run/shiftzilla/config.yaml"); err != nil {
		klog.Warning("Shiftzilla Configuration not found")
		return
	}
	if _, err := os.Stat("/var/run/people/config.yaml"); err != nil {
		klog.Warning("People Configuration not found")
		return
	}

	teams, err := DecodeTeamComponents("/var/run/shiftzilla/config.yaml")
	if err != nil {
		klog.Warning(err.Error())
		return
	}
	people, err := DecodePeopleList("/var/run/people/config.yaml")
	if err != nil {
		klog.Warning(err.Error())
		return
	}

	charter := NewCharter(*people, *teams)

	slackerInstance.Command("who-own <component>", &slacker.CommandDefinition{
		Description: "Display information about who own the component.",
		Handler: func(req slacker.Request, w slacker.ResponseWriter) {
			component := req.StringParam("component", "")
			result := []string{}
			if team := charter.LookupComponentTeam(component); team != nil {
				result = append(result, fmt.Sprintf("Component %q is owned by %s team", component, team.Name))
			}
			if maintainers := charter.LookupComponentMaintainers(component); maintainers != nil {
				result = append(result, "List of maintainers:\n")
				for _, p := range maintainers {
					result = append(result, fmt.Sprintf("> %s (:github: %s, :email: %s, timezone: %s)", p.Name, p.Github, p.Email, p.Timezone))
				}
			}
			if len(result) == 0 {
				components := charter.LookupMatchingComponentName(component)
				if len(components) > 0 {
					result = append(result, fmt.Sprintf("I don't know component %q, but I found these components that might be related: %s", component, strings.Join(components, ", ")))
				} else {
					result = append(result, ":sadpanda: Sorry, I don't know who own component %q", component)
				}
			}
			w.Reply(strings.Join(result, "\n"))
		},
	})

	slackerInstance.Command("who-is <name>", &slacker.CommandDefinition{
		Description: "Lookup a person name.",
		Handler: func(req slacker.Request, w slacker.ResponseWriter) {
			name := req.StringParam("name", "")
			result := []string{}
			if people := charter.LookupMatchingPersonName(name); len(people) > 0 {
				for _, p := range people {
					result = append(result, fmt.Sprintf("> %s is member of %q team (:github: %s, :email: %s, timezone: %s)", p.Name, p.Team, p.Github, p.Email, p.Timezone))
				}
			} else {
				result = append(result, ":sadpanda: Sorry, I don't know who %q is", name)
			}
			w.Reply(strings.Join(result, "\n"))
		},
	})
}
