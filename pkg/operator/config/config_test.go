package config

import (
	"testing"
)

func TestBugzillaCredentials_Obfuscate(t *testing.T) {
	c := Credentials{
		Username: "test",
		Password: "base64:Zm9v",
		APIKey:   "",
	}
	if user := c.DecodedUsername(); user != "test" {
		t.Errorf("expected username to be 'test', got %q", user)
	}
	if password := c.DecodedPassword(); password != "foo" {
		t.Errorf("expected password to be 'foo', got %q", password)
	}
	if key := c.DecodedAPIKey(); key != "" {
		t.Errorf("expected password to be empty, got %q", key)
	}
}
