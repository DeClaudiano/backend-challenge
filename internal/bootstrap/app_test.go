package bootstrap

import (
	"testing"

	"go.uber.org/fx"
)

func TestAPIOptionBuildsDependencyGraph(t *testing.T) {
	if err := fx.ValidateApp(API()); err != nil {
		t.Fatal(err)
	}
}
