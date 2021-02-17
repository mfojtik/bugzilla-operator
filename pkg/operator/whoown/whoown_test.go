package whoown

import "testing"

func TestNewTracker(t *testing.T) {
	tracker, err := NewTracker("./members.csv")
	if err != nil {
		t.Fatal(err)
	}
	r := tracker.Component("oc")
	if len(r) == 0 {
		t.Fatalf("expected at least one record, got none")
	}
}

func TestTracker_Component(t *testing.T) {
	tracker, err := NewTracker("./members.csv")
	if err != nil {
		t.Fatal(err)
	}

	r := tracker.Component("oc")
	if len(r) == 0 {
		t.Fatalf("expected at least one record, got none")
	}

	if expected := `owned by "Workloads" team. You can find them in #forum-workloads (or use slack handle @team-workloads).`; r[0].String() != expected {
		t.Errorf("expected descrition %q does not match returned %q", expected, r[0].String())
	}

	r = tracker.Component("foo")
	if len(r) != 0 {
		t.Errorf("expected no components matches, got %#v", r)
	}
}

func TestTracker_Description(t *testing.T) {
	tracker, err := NewTracker("./members.csv")
	if err != nil {
		t.Fatal(err)
	}

	r := tracker.Description("scheduler")
	if len(r) == 0 {
		t.Fatalf("expected at least one record, got none")
	}

	if expected := `owned by "Workloads" team. You can find them in #forum-workloads (or use slack handle @team-workloads).`; r[0].String() != expected {
		t.Errorf("expected descrition %q does not match returned %q", expected, r[0].String())
	}

	r = tracker.Component("foo")
	if len(r) != 0 {
		t.Errorf("expected no components matches, got %#v", r)
	}
}
