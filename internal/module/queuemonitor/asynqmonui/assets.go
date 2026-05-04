package asynqmonui

import "embed"

// Build contains the official asynqmon UI build files.
//
// The files are copied from github.com/hibiken/asynqmon@v0.7.2/ui/build
// because asynqmon's built-in static handler calls filepath.Abs on URL paths,
// which returns Windows drive paths and breaks RootPath prefix matching on
// Windows. API handling still belongs to the official asynqmon handler.
//
//go:embed build/*
var Build embed.FS
