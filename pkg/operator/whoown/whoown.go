package whoown

import (
	"encoding/csv"
	"fmt"
	"os"
	"strings"
)

type Tracker struct {
	records []*Record
}

type Record struct {
	PublicChannelName string
	SlackHandle       string
	TeamName          string

	keywords    []string
	components  []string
	description string
}

func (r Record) String() string {
	handle := ""
	if len(r.SlackHandle) > 0 {
		handle = fmt.Sprintf(" (or use slack handle %s)", r.SlackHandle)
	}
	return fmt.Sprintf("owned by %q team. You can find them in %s%s.", r.TeamName, r.PublicChannelName, handle)
}

func NewTracker(teamMemberTrackingCSVFile string) (*Tracker, error) {
	file, err := os.Open(teamMemberTrackingCSVFile)
	if err != nil {
		return nil, err
	}
	r := csv.NewReader(file)
	records, err := r.ReadAll()
	if err != nil {
		return nil, err
	}

	owners := []*Record{}
	for _, r := range records {
		// only track team with public slack forum channel
		if len(r) < 13 || len(r[13]) == 0 || !strings.HasPrefix(r[13], "#") {
			continue
		}
		newRecord := &Record{}
		newRecord.TeamName = r[1]
		newRecord.description = r[11]
		newRecord.PublicChannelName = r[13]
		if len(r) >= 14 {
			newRecord.SlackHandle = r[14]
		}
		if len(r) >= 16 {
			newRecord.components = []string{r[16]}
		}
		if len(r) >= 18 {
			newRecord.keywords = []string{r[18]}
		}
		owners = append(owners, newRecord)
	}
	return &Tracker{records: owners}, nil
}

// Component search all team bugzilla components and return records that contain the given name in component list.
func (t Tracker) Component(name string) []Record {
	matching := []Record{}
	for _, r := range t.records {
		for _, c := range r.components {
			if !strings.Contains(c, name) {
				continue
			}
			matching = append(matching, *r)
		}
	}
	return matching
}

// Description search all team description and return records matching the name.
func (t Tracker) Description(name string) []Record {
	matching := []Record{}
	for _, r := range t.records {
		if !strings.Contains(r.description, name) {
			continue
		}
		matching = append(matching, *r)
	}
	return matching
}

// Keywords search all team keywords and return records matching the name.
func (t Tracker) Keywords(name string) []Record {
	matching := []Record{}
	for _, r := range t.records {
		for _, c := range r.keywords {
			if !strings.Contains(c, name) {
				continue
			}
			matching = append(matching, *r)
		}
	}
	return matching
}
