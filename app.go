package main

import (
	"context"
)

// App struct
type App struct {
	ctx context.Context
}

// NewApp creates a new App application struct
func NewApp() *App {
	return &App{}
}

// startup is called when the app starts. The context is saved
// so we can call the runtime methods
func (a *App) startup(ctx context.Context) {
	a.ctx = ctx
}

// Ping is the smoke-test method for task 0.3: confirms the full
// frontend-to-Go IPC round trip and Wails' generated TS bindings work.
func (a *App) Ping() string {
	return "pong"
}
