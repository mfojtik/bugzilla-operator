package config

import (
	"encoding/base64"
	"strings"
)

type Credentials struct {
	Username               string `yaml:"username"`
	Password               string `yaml:"password"`
	APIKey                 string `yaml:"apiKey"`
	SlackToken             string `yaml:"slackToken"`
	SlackVerificationToken string `yaml:"slackVerificationToken"`
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
	Stale    BugzillaList `yaml:"stale"`
	Blockers BugzillaList `yaml:"blockers"`
	Closed   BugzillaList `yaml:"closed"`
}

type Transition struct {
	From string `yaml:"from"`
	To   string `yaml:"to"`
}

type BugzillaRelease struct {
	CurrentTargetRelease string `yaml:"currentTargetRelease"`
}

type OperatorConfig struct {
	Credentials Credentials   `yaml:"credentials"`
	Lists       BugzillaLists `yaml:"lists"`

	Release BugzillaRelease `yaml:"release"`

	// SlackChannel is a channel where the operator will post reports/etc.
	SlackChannel string `yaml:"slackChannel"`

	// SlackUserEmail represents a Slack user email the events will be sent to
	SlackUserEmail string `yaml:"slackUserEmail"`
}

// Anonymize makes a shallow copy of the config, suitable for dumping in logs (no sensitive data)
func (c *OperatorConfig) Anonymize() OperatorConfig {
	a := *c
	if user := a.Credentials.Username; len(user) > 0 {
		a.Credentials.Username = strings.Repeat("x", len(a.Credentials.DecodedUsername()))
	}
	if password := a.Credentials.Password; len(password) > 0 {
		a.Credentials.Password = strings.Repeat("x", len(a.Credentials.DecodedPassword()))
	}
	if key := a.Credentials.APIKey; len(key) > 0 {
		a.Credentials.APIKey = strings.Repeat("x", len(a.Credentials.DecodedAPIKey()))
	}
	if key := a.Credentials.SlackToken; len(key) > 0 {
		a.Credentials.SlackToken = strings.Repeat("x", len(a.Credentials.DecodedSlackToken()))
	}
	if key := a.Credentials.SlackVerificationToken; len(key) > 0 {
		a.Credentials.SlackVerificationToken = strings.Repeat("x", len(a.Credentials.DecodedSlackVerificationToken()))
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
func (b Credentials) DecodedAPIKey() string {
	return decode(b.APIKey)
}

// DecodedAPIKey return decoded Password (in case it was base64 encoded)
func (b Credentials) DecodedPassword() string {
	return decode(b.Password)
}

// DecodedAPIKey return decoded Username (in case it was base64 encoded)
func (b Credentials) DecodedUsername() string {
	return decode(b.Username)
}

func (b Credentials) DecodedSlackToken() string {
	return decode(b.SlackToken)
}

func (b Credentials) DecodedSlackVerificationToken() string {
	return decode(b.SlackVerificationToken)
}
