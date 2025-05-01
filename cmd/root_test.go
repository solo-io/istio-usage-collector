//go:build test || unit

package cmd

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// TestRootCommandFlags verifies that flags are available on the root command
func TestRootCommandFlags(t *testing.T) {
	cmd := GetCommand()
	assert.NotNil(t, cmd.Flag("hide-names"))
	assert.NotNil(t, cmd.Flag("continue"))
	assert.NotNil(t, cmd.Flag("context"))
	assert.NotNil(t, cmd.Flag("output-dir"))
	assert.NotNil(t, cmd.Flag("format"))
	assert.NotNil(t, cmd.Flag("output-prefix"))
	assert.NotNil(t, cmd.Flag("debug"))
	assert.NotNil(t, cmd.Flag("no-progress"))
	assert.NotNil(t, cmd.Flag("max-processors"))

	assert.Nil(t, cmd.Flag("version")) // This is only set for builds in standalone mode, not part of the command in general
}

// TestRootCommandFlagParsing verifies that flags are parsed correctly *without* executing RunE.
// Note: When executing RunE, unit tests become unreliable due to this tool using actual kubectl commands so we can't test against
// any contexts, as they may not exist. Execution tests are handled in e2e tests.
func TestRootCommandFlagParsing(t *testing.T) {
	// Test parsing of flags
	tests := []struct {
		name          string
		args          []string
		expectedFlags CommandFlags // Expected values *after* parsing
	}{
		{
			name: "Default flags",
			args: []string{},
			expectedFlags: CommandFlags{
				HideNames:          false,
				ContinueProcessing: false,
				KubeContext:        "", // Default is empty string, where during execution, the current context is used
				OutputDir:          ".",
				OutputFormat:       "json",
				OutputFilePrefix:   "", // Default is empty string, where during execution, the context name is used
				EnableDebug:        false,
				NoProgress:         false,
				MaxProcessors:      0,
			},
		},
		{
			name: "All flags set",
			args: []string{
				"--hide-names",
				"--continue",
				"--context", "test-context",
				"--output-dir", "/tmp/test",
				"--format", "yaml",
				"--output-prefix", "my-prefix",
				"--debug",
				"--no-progress",
				"--max-processors", "1",
			},
			expectedFlags: CommandFlags{
				HideNames:          true,
				ContinueProcessing: true,
				KubeContext:        "test-context",
				OutputDir:          "/tmp/test",
				OutputFormat:       "yaml",
				OutputFilePrefix:   "my-prefix",
				EnableDebug:        true,
				NoProgress:         true,
				MaxProcessors:      1,
			},
		},
		{
			name: "Short flags",
			args: []string{
				"-n",
				"-c",
				"-k", "short-ctx",
				"-d", "/short",
				"-f", "yml",
				"-p", "short-p",
			},
			expectedFlags: CommandFlags{
				HideNames:          true,
				ContinueProcessing: true,
				KubeContext:        "short-ctx",
				OutputDir:          "/short",
				OutputFormat:       "yml",
				OutputFilePrefix:   "short-p",
				EnableDebug:        false, // default
				NoProgress:         false, // default
				MaxProcessors:      0,     // default
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rootCmd := GetCommand()

			// Use ParseFlags to only process flags, not execute RunE
			err := rootCmd.ParseFlags(tt.args)
			assert.NoError(t, err)

			cmdFlags := rootCmd.Flags()

			hideNames, err := cmdFlags.GetBool("hide-names")
			assert.NoError(t, err)
			assert.Equal(t, tt.expectedFlags.HideNames, hideNames, "Flag HideNames mismatch")

			continueProcessing, err := cmdFlags.GetBool("continue")
			assert.NoError(t, err)
			assert.Equal(t, tt.expectedFlags.ContinueProcessing, continueProcessing, "Flag ContinueProcessing mismatch")

			kubeContext, err := cmdFlags.GetString("context")
			assert.NoError(t, err)
			assert.Equal(t, tt.expectedFlags.KubeContext, kubeContext, "Flag KubeContext mismatch")

			outputDir, err := cmdFlags.GetString("output-dir")
			assert.NoError(t, err)
			assert.Equal(t, tt.expectedFlags.OutputDir, outputDir, "Flag OutputDir mismatch")

			outputFormat, err := cmdFlags.GetString("format")
			assert.NoError(t, err)
			assert.Equal(t, tt.expectedFlags.OutputFormat, outputFormat, "Flag OutputFormat mismatch")

			outputFilePrefix, err := cmdFlags.GetString("output-prefix")
			assert.NoError(t, err)
			assert.Equal(t, tt.expectedFlags.OutputFilePrefix, outputFilePrefix, "Flag OutputFilePrefix mismatch")

			enableDebug, err := cmdFlags.GetBool("debug")
			assert.NoError(t, err)
			assert.Equal(t, tt.expectedFlags.EnableDebug, enableDebug, "Flag EnableDebug mismatch")

			noProgress, err := cmdFlags.GetBool("no-progress")
			assert.NoError(t, err)
			assert.Equal(t, tt.expectedFlags.NoProgress, noProgress, "Flag NoProgress mismatch")

			maxProcessors, err := cmdFlags.GetInt("max-processors")
			assert.NoError(t, err)
			assert.Equal(t, tt.expectedFlags.MaxProcessors, maxProcessors, "Flag MaxProcessors mismatch")
		})
	}
}
