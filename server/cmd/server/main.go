package main

import (
	"go.uber.org/fx"

	"github.com/PineappleBond/eino-demo-dev/server/internal/di"
)

func main() {
	app := fx.New(
		di.Module,
	)
	app.Run()
}
