package config

import (
	"encoding/base64"
	"strings"
)

type BugzillaCredentials struct {
	Username string `yaml:"username"`
	Password string `yaml:"password"`
	APIKey   string `yaml:"apiKey"`
}

type BugzillaList struct {
	Name     string    `yaml:"name"`
	SharerID string    `yaml:"sharerID"`
	Action   BugAction `yaml:"action"`
}

type BugAction struct {
	AddComment           string       `yaml:"addComment"`
	AddKeyword           string       `yaml:"addKeyword"`
	PriorityTransitions  []Transition `yaml:"priorityTransitions"`
	SeverityTransitions  []Transition `yaml:"severityTransitions"`
	NeedInfoFromCreator  bool         `yaml:"needInfoFromCreator"`
	NeedInfoFromAssignee bool         `yaml:"needInfoFromAssignee"`
}

type BugzillaLists struct {
	Stale BugzillaList `yaml:"stale"`
}

type Transition struct {
	From string `yaml:"from"`
	To   string `yaml:"to"`
}

type OperatorConfig struct {
	Credentials BugzillaCredentials `yaml:"credentials"`
	Lists       BugzillaLists       `yaml:"lists"`
}

// Anonymize makes a shallow copy of the config, suitable for dumping in logs (no sensitive data)
func (c *OperatorConfig) Anonymize() OperatorConfig {
	a := *c
	if user := a.Credentials.Username; len(user) > 0 {
		a.Credentials.Username = "<set>"
	}
	if password := a.Credentials.Password; len(password) > 0 {
		a.Credentials.Password = "<set>"
	}
	if key := a.Credentials.Username; len(key) > 0 {
		a.Credentials.APIKey = "<set>"
	}
	return a
}

func decode(s string) string {
	if strings.HasPrefix(s, "base64:") {
		data, err := base64.StdEncoding.DecodeString(strings.TrimPrefix(s, "base64:"))
		if err != nil {
			return s
		}
		return string(data)
	}
	return s
}

// DecodedAPIKey return decoded APIKey (in case it was base64 encoded)
func (b BugzillaCredentials) DecodedAPIKey() string {
	return decode(b.APIKey)
}

// DecodedAPIKey return decoded Password (in case it was base64 encoded)
func (b BugzillaCredentials) DecodedPassword() string {
	return decode(b.Password)
}

// DecodedAPIKey return decoded Username (in case it was base64 encoded)
func (b BugzillaCredentials) DecodedUsername() string {
	return decode(b.Username)
}
