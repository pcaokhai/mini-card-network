package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestWalk_skipsAllowlistedFixtureButFlagsOthers__MCN_506_AC1(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(dir, "contracts", "fixtures"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "contracts", "fixtures", "cards.json"), []byte(`{"pan":"9704360000004417"}`), 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "app.log"), []byte("card 4111111111111111 approved\n"), 0o600))

	findings, err := Walk([]string{dir}, []string{"contracts/fixtures/cards.json"})

	require.NoError(t, err)
	require.Len(t, findings, 1)
	require.Equal(t, "app.log", filepath.Base(findings[0].File))
}
