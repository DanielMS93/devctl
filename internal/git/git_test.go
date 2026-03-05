package git

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseWorktrees_MainPlusLinked(t *testing.T) {
	fixture := []byte("worktree /path/to/main\nHEAD abc123def456\nbranch refs/heads/main\n\n" +
		"worktree /path/to/feature\nHEAD def456abc789\nbranch refs/heads/feature/login\n\n")
	wts := parseWorktrees(fixture)
	require.Len(t, wts, 2)
	assert.Equal(t, "/path/to/main", wts[0].Path)
	assert.Equal(t, "main", wts[0].Branch)
	assert.False(t, wts[0].Locked)
	assert.Equal(t, "/path/to/feature", wts[1].Path)
	assert.Equal(t, "feature/login", wts[1].Branch)
}

func TestParseWorktrees_LockedWorktree(t *testing.T) {
	fixture := []byte("worktree /path/to/locked\nHEAD abc123\nbranch refs/heads/fix/bug\nlocked temp lock\n\n")
	wts := parseWorktrees(fixture)
	require.Len(t, wts, 1)
	assert.True(t, wts[0].Locked)
}

func TestParseWorktrees_DetachedHead(t *testing.T) {
	fixture := []byte("worktree /path/to/detached\nHEAD abc123\ndetached\n\n")
	wts := parseWorktrees(fixture)
	require.Len(t, wts, 1)
	assert.Equal(t, "", wts[0].Branch) // no branch line = empty
}

func TestParseStatus_WithUpstream(t *testing.T) {
	fixture := []byte(
		"# branch.oid abc123\n" +
			"# branch.head main\n" +
			"# branch.upstream origin/main\n" +
			"# branch.ab +2 -1\n" +
			"1 M. N... 100644 100644 100644 abc def file.go\n" +
			"1 .M N... 100644 100644 100644 abc def other.go\n" +
			"? untracked.txt\n",
	)
	state := parseStatus(fixture)
	assert.Equal(t, 2, state.Ahead)
	assert.Equal(t, 1, state.Behind)
	assert.Equal(t, 1, state.Staged)   // M. -> X='M' staged
	assert.Equal(t, 1, state.Unstaged) // .M -> Y='M' unstaged
	assert.Equal(t, 1, state.Untracked)
	assert.Len(t, state.ChangedFiles, 2)
}

func TestParseStatus_NoUpstream(t *testing.T) {
	fixture := []byte(
		"# branch.oid abc123\n" +
			"# branch.head feature\n",
	)
	state := parseStatus(fixture)
	assert.Equal(t, 0, state.Ahead)
	assert.Equal(t, -1, state.Behind) // sentinel: no upstream
	assert.Equal(t, 0, state.Staged)
	assert.Equal(t, 0, state.Unstaged)
	assert.Equal(t, 0, state.Untracked)
}

func TestParseStatus_CleanWithUpstream(t *testing.T) {
	fixture := []byte(
		"# branch.oid abc123\n" +
			"# branch.head main\n" +
			"# branch.upstream origin/main\n" +
			"# branch.ab +0 -0\n",
	)
	state := parseStatus(fixture)
	assert.Equal(t, 0, state.Ahead)
	assert.Equal(t, 0, state.Behind)
	assert.Empty(t, state.ChangedFiles)
}
