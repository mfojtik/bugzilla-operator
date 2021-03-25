package teamcontroller

import (
	"io/ioutil"
	"strings"

	"gopkg.in/yaml.v2"
)

// PeopleList contain list of people copied from OpenShift organization spreadsheet.
type PeopleList []Person

// Person contain information about single engineer
type Person struct {
	Name     string `yaml:"name"`
	Pillar   string `yaml:"pillar"`
	Team     string `yaml:"team"`
	Role     string `yaml:"role"`
	Email    string `yaml:"email"`
	Github   string `yaml:"github"`
	Timezone string `yaml:"timezone"`
}

// TeamComponents contain list of teams. Each team lists components it owns.
// The team name here matches the Person->Team.
type TeamComponents struct {
	Teams []Team `yaml:"Teams"`
}

// Team represents a single team and its components.
type Team struct {
	Name       string   `yaml:"name"`
	Components []string `yaml:"components"`
	Slack      string   `yaml:"slack_chan"`
}

// Charter holds team configuration and people list.
type Charter struct {
	Teams  []Team
	People []Person
}

// NewCharter return new charter.
func NewCharter(people PeopleList, teams TeamComponents) *Charter {
	return &Charter{
		Teams:  teams.Teams,
		People: people,
	}
}

// LookupComponentTeam looks up a team based on the component name.
// If team does not exists, this returns nil.
func (c *Charter) LookupComponentTeam(name string) *Team {
	for _, t := range c.Teams {
		for _, c := range t.Components {
			if strings.ToLower(c) == strings.ToLower(name) {
				return &t
			}
		}
	}
	return nil
}

// LookupMatchingComponentName return list of component that match the name.
func (c *Charter) LookupMatchingComponentName(name string) []string {
	result := []string{}
	for _, t := range c.Teams {
		for _, c := range t.Components {
			if strings.Contains(strings.ToLower(c), strings.ToLower(name)) {
				result = append(result, c)
			}
		}
	}
	return result
}

// LookupComponentMaintainers return list of engineers that are responsible for component maintenance.
func (c *Charter) LookupComponentMaintainers(name string) []Person {
	team := c.LookupComponentTeam(name)
	if team == nil {
		return nil
	}
	var people []Person
	for _, p := range c.People {
		if strings.ToLower(p.Team) == strings.ToLower(team.Name) {
			people = append(people, p)
		}
	}
	return people
}

// LookupPersonTeam finds a team a person belongs to.
func (c *Charter) LookupPersonTeam(name string) *Team {
	var person *Person
	for i, p := range c.People {
		if strings.ToLower(p.Name) == strings.ToLower(name) {
			person = &c.People[i]
			break
		}
	}
	if person == nil {
		return nil
	}
	for _, t := range c.Teams {
		if strings.ToLower(t.Name) == strings.ToLower(person.Team) {
			return &t
		}
	}
	return nil
}

// LookupMatchingPersonName finds people matching the given name.
func (c *Charter) LookupMatchingPersonName(name string) []Person {
	var people []Person
	for _, p := range c.People {
		if strings.Contains(strings.ToLower(p.Name), strings.ToLower(name)) {
			people = append(people, p)
		}
	}
	return people
}

func DecodePeopleList(path string) (*PeopleList, error) {
	result := PeopleList{}
	peopleBytes, err := ioutil.ReadFile(path)
	if err != nil {
		return nil, err
	}
	if err := yaml.Unmarshal(peopleBytes, &result); err != nil {
		return nil, err
	}
	return &result, nil
}

func DecodeTeamComponents(path string) (*TeamComponents, error) {
	result := TeamComponents{}
	teamsBytes, err := ioutil.ReadFile(path)
	if err != nil {
		return nil, err
	}
	if err := yaml.Unmarshal(teamsBytes, &result); err != nil {
		return nil, err
	}
	return &result, nil
}
