package teamcontroller

import (
	"testing"

	"k8s.io/apimachinery/pkg/util/sets"
)

func TestPeople(t *testing.T) {
	people, err := DecodePeopleList("sample_people_list.yaml")
	if err != nil {
		t.Fatalf("failed to read team_config.yaml: %v", err)
	}
	teams, err := DecodeTeamComponents("sample_shiftzilla_conf.yaml")
	if err != nil {
		t.Fatalf("failed to read shiftzilla_cfg.yaml: %v", err)
	}

	charter := NewCharter(*people, *teams)

	maintainers := charter.LookupComponentMaintainers("config-operator")
	if len(maintainers) == 0 || maintainers[0].Name != "Stefan Schimanski" {
		t.Errorf("expected Stefan to own config-operator, got %q", maintainers[0].Name)
	}

	components := charter.LookupMatchingComponentName("operator")
	componentSet := sets.NewString(components...)
	if !componentSet.Has("config-operator") {
		t.Errorf("expected to have config-operator")
	}
	if componentSet.Len() != 2 {
		t.Errorf("expected 2 operators")
	}

	david := charter.LookupMatchingPersonName("david eads")
	if david[0].Name != "David Eads" {
		t.Errorf("expected David to have proper name")
	}

	apiTeam := charter.LookupPersonTeam(david[0].Name)
	if apiTeam.Name != "API Server & Auth" {
		t.Errorf("expected api&auth to have proper name, got %q", apiTeam.Name)
	}
}
