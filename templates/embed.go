// Package templates holds embedded Clean Architecture scaffolds.
package templates

import "embed"

// FS contains the go and node project trees.
//
//go:embed all:go all:node
var FS embed.FS
