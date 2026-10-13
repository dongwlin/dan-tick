package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func gitTest(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	data, err := cmd.CombinedOutput()
	require.NoError(t, err, "%s", data)
	return strings.TrimSpace(string(data))
}

func testRepo(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	gitTest(t, dir, "init", "-b", "main")
	gitTest(t, dir, "config", "user.name", "Build Test")
	gitTest(t, dir, "config", "user.email", "build-test@example.invalid")
	gitTest(t, dir, "commit", "--allow-empty", "-m", "initial")
	return dir
}

func env(values map[string]string) func(string) string {
	return func(key string) string { return values[key] }
}

func TestVersionReleaseAndHistory(t *testing.T) {
	for _, annotated := range []bool{false, true} {
		t.Run(map[bool]string{false: "lightweight", true: "annotated"}[annotated], func(t *testing.T) {
			dir := testRepo(t)
			args := []string{"tag", "v1.2.3"}
			if annotated {
				args = append(args, "-a", "-m", "release")
			}
			gitTest(t, dir, args...)
			head := gitTest(t, dir, "rev-parse", "HEAD")
			values := map[string]string{"GITHUB_ACTIONS": "true", "GITHUB_EVENT_NAME": "push", "GITHUB_REF_TYPE": "tag", "GITHUB_REF_NAME": "v1.2.3"}
			info, err := resolveVersion(dir, env(values), "0.0.0")
			require.NoError(t, err)
			require.Equal(t, versionInfo{"v1.2.3", "1.2.3", head, true}, info)
			gitTest(t, dir, "commit", "--allow-empty", "-m", "next")
			gitTest(t, dir, "tag", "test-current")
			gitTest(t, dir, "tag", "v01.2.3")
			gitTest(t, dir, "tag", "v2.0.0-rc.1")
			values["GITHUB_REF_TYPE"] = "branch"
			info, err = resolveVersion(dir, env(values), "0.0.0")
			require.NoError(t, err)
			require.Equal(t, "v1.2.3-"+info.Commit[:8], info.Version)
			require.Equal(t, "1.2.3", info.PackageVersion)
			require.False(t, info.Release)
			// A newer release on another branch is not an ancestor baseline.
			gitTest(t, dir, "checkout", "-b", "other", "HEAD~1")
			gitTest(t, dir, "commit", "--allow-empty", "-m", "unrelated")
			gitTest(t, dir, "tag", "v9.0.0")
			gitTest(t, dir, "checkout", "main")
			info, err = resolveVersion(dir, env(values), "0.0.0")
			require.NoError(t, err)
			require.Equal(t, "v1.2.3-"+info.Commit[:8], info.Version)
		})
	}
}

func TestVersionLocalFirstCIAndNoGit(t *testing.T) {
	dir := testRepo(t)
	info, err := resolveVersion(dir, env(map[string]string{"CI": "true"}), "4.5.6")
	require.NoError(t, err)
	require.Equal(t, "dev-"+info.Commit[:8], info.Version)
	require.Equal(t, "4.5.6", info.PackageVersion)
	gitTest(t, dir, "tag", "v4.5.6")
	info, err = resolveVersion(dir, env(nil), "4.5.6")
	require.NoError(t, err)
	require.Equal(t, "dev", info.Version)
	require.Equal(t, "4.5.6", info.PackageVersion)
	for _, values := range []map[string]string{nil, {"CI": "true"}} {
		info, err = resolveVersion(t.TempDir(), env(values), "0.0.0")
		require.NoError(t, err)
		require.Equal(t, "dev", info.Version)
		require.Empty(t, info.Commit)
		require.False(t, info.Release)
	}
}

func TestReleaseRequiresExactEventAndCommit(t *testing.T) {
	dir := testRepo(t)
	gitTest(t, dir, "tag", "v1.0.0")
	for _, event := range []string{"workflow_dispatch", "pull_request", "push"} {
		for _, refType := range []string{"branch", "tag"} {
			info, err := resolveVersion(dir, env(map[string]string{"GITHUB_ACTIONS": "true", "GITHUB_EVENT_NAME": event, "GITHUB_REF_TYPE": refType, "GITHUB_REF_NAME": "v1.0.0"}), "0.0.0")
			require.NoError(t, err)
			require.Equal(t, event == "push" && refType == "tag", info.Release)
		}
	}
	gitTest(t, dir, "commit", "--allow-empty", "-m", "not tagged")
	_, err := resolveVersion(dir, env(map[string]string{"GITHUB_ACTIONS": "true", "GITHUB_EVENT_NAME": "push", "GITHUB_REF_TYPE": "tag", "GITHUB_REF_NAME": "v1.0.0"}), "0.0.0")
	require.ErrorContains(t, err, "does not resolve to Git HEAD")
}

func TestStrictTagAndNumericResourceVersion(t *testing.T) {
	for _, tag := range []string{"v0.0.0", "v1.2.3", "v100.200.300"} {
		require.True(t, releaseTag.MatchString(tag))
	}
	for _, tag := range []string{"1.2.3", "v01.2.3", "v1.02.3", "v1.2.03", "v1.2", "v1.2.3.4", "v1.2.3-rc.1", "v1.2.3+build", "v1.2.3\n"} {
		require.False(t, releaseTag.MatchString(tag), tag)
	}
	for _, version := range []string{"1.2.3", "65535.0.0"} {
		fixed, err := parsePackageVersion(version)
		require.NoError(t, err)
		require.Equal(t, version+".0", fixed)
	}
	for _, version := range []string{"v1.2.3", "1.2.3-abcdef12", "1.2.3.4", "65536.0.0"} {
		_, err := parsePackageVersion(version)
		require.Error(t, err)
	}
}

func TestMetadataUsesCompileTimeUTCAndOverrides(t *testing.T) {
	commit := strings.Repeat("a", 40)
	info, err := resolveVersion(t.TempDir(), env(map[string]string{"DAN_TICK_VERSION": "v1.2.3-aabbccdd", "DAN_TICK_COMMIT": commit}), "0.0.0")
	require.NoError(t, err)
	require.Equal(t, "1.2.3", info.PackageVersion)
	compiledAt := time.Date(2026, 10, 13, 18, 1, 2, 0, time.FixedZone("Shanghai", 8*3600))
	flags := metadataFlags(info, compiledAt)
	require.Contains(t, flags, "Version=v1.2.3-aabbccdd")
	require.Contains(t, flags, "BuildTime=2026-10-13T10:01:02Z")
	require.Contains(t, flags, "Commit="+commit)
}

func TestPrepareDistPreservesExistingBuild(t *testing.T) {
	path := filepath.Join(t.TempDir(), "dist", "index.html")
	require.NoError(t, prepareDist(path))
	require.NoError(t, os.WriteFile(path, []byte("real build"), 0644))
	require.NoError(t, prepareDist(path))
	data, err := os.ReadFile(path)
	require.NoError(t, err)
	require.Equal(t, "real build", string(data))
}
