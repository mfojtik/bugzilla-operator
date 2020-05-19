package operator

import (
	"fmt"
	"reflect"
	"testing"

	"k8s.io/apimachinery/pkg/util/sets"

	"github.com/mfojtik/bugzilla-operator/pkg/operator/config"
)

func TestExpandGroup(t *testing.T) {
	type args struct {
		cfg   map[string]config.Group
		x     string
		users sets.String
	}
	tests := []struct {
		cfg   map[string]config.Group
		x     string
		users sets.String
		want  sets.String
	}{
		{nil, "a", sets.String{}, sets.NewString("a")},
		{nil, "a", sets.NewString("a"), sets.NewString("a")},
		{nil, "a", sets.NewString("b"), sets.NewString("a", "b")},
		{map[string]config.Group{
			"A": []string{"a", "b"},
		}, "group:A", sets.String{}, sets.NewString("a", "b")},
		{map[string]config.Group{
			"A": []string{"a", "b"},
			"B": []string{"b", "c"},
		}, "group:C", sets.String{}, sets.NewString()},
		{map[string]config.Group{
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
