package main

import (
	"backend-challenge/internal/bootstrap"
	"go.uber.org/fx"
)

func main() { fx.New(bootstrap.API()).Run() }
