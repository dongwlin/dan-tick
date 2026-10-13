package buildinfo

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestCurrentPreservesCompiledMetadataAndReturnsAnIndependentSnapshot(t *testing.T) {
	original := Current()
	t.Cleanup(func() { Version, BuildTime, Commit = original.Version, original.BuildTime, original.Commit })
	Version, BuildTime, Commit = "v1.2.3-a1b2c3d4", "2026-10-13T08:12:00Z", "a1b2c3d4e5f67890"

	info := Current()
	require.Equal(t, Info{
		Version: "v1.2.3-a1b2c3d4", BuildTime: "2026-10-13T08:12:00Z", Commit: "a1b2c3d4e5f67890",
	}, info)
	info.Version = "changed by a caller"
	require.Equal(t, "v1.2.3-a1b2c3d4", Current().Version)
}
