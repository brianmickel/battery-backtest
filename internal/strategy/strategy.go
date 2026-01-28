package strategy

import "battery-backtest/internal/model"

type Context struct {
	Index int
	Interval model.LMPInterval
	Battery  *model.Battery
}

type Strategy interface {
	Name() string
	Decide(ctx Context) model.Dispatch
}

