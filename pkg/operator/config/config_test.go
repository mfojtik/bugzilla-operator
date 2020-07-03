package config

import (
	"fmt"
	"reflect"
	"testing"

	"k8s.io/apimachinery/pkg/util/sets"
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

func TestExpandGroup(t *testing.T) {
	tests := []struct {
		cfg   map[string]Group
		x     string
		users sets.String
		want  sets.String
	}{
		{nil, "a", sets.String{}, sets.NewString("a")},
		{nil, "a", sets.NewString("a"), sets.NewString("a")},
		{nil, "a", sets.NewString("b"), sets.NewString("a", "b")},
		{map[string]Group{
			"A": []string{"a", "b"},
		}, "group:A", sets.String{}, sets.NewString("a", "b")},
		{map[string]Group{
			"A": []string{"a", "b"},
			"B": []string{"b", "c"},
		}, "group:C", sets.String{}, sets.NewString()},
		{map[string]Group{
			"A": []string{"a"},
			"B": []string{"b", "group:A"},
			"C": []string{"c", "group:B"},
			"D": []string{"d", "group:C", "group:C", "group:E"},
			"F": []string{"f"},
		}, "group:D", sets.NewString("e"), sets.NewString("a", "b", "c", "d", "e")},
	}
	for i, tt := range tests {
		t.Run(fmt.Sprintf("%d", i), func(t *testing.T) {
			if got, _ := expandGroup(tt.cfg, tt.x, tt.users, nil); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("expandGroup() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestExpandGroups(t *testing.T) {
	tests := []struct {
		cfg  map[string]Group
		x    string
		want sets.String
	}{
		{nil, "a", sets.NewString("a")},
		{map[string]Group{
			"A": []string{"a", "b"},
		}, "group:A", sets.NewString("a", "b")},
		{map[string]Group{
			"A": []string{"a", "b"},
			"B": []string{"b", "c"},
		}, "group:C", sets.NewString()},
		{map[string]Group{
			"A": []string{"a"},
			"B": []string{"b", "group:A"},
			"C": []string{"c", "group:B"},
			"D": []string{"d", "group:C", "group:C", "group:E"},
			"F": []string{"f"},
		}, "group:D", sets.NewString("a", "b", "c", "d")},
	}
	for i, tt := range tests {
		t.Run(fmt.Sprintf("%d", i), func(t *testing.T) {
			if got := ExpandGroups(tt.cfg, tt.x); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("expandGroup() = %v, want %v", got, tt.want)
			}
		})
	}
}
