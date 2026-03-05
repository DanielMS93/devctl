//go:build tools

// tools.go retains build-tool and future-use dependencies in go.mod so they
// are available when needed in subsequent plans without requiring re-fetching.
package main

import (
	_ "charm.land/bubbles/v2"
	_ "charm.land/bubbletea/v2"
	_ "charm.land/lipgloss/v2"
	_ "github.com/spf13/cobra"
	_ "github.com/spf13/viper"
	_ "github.com/stretchr/testify/assert"
)
