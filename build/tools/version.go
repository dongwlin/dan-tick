package main

import (
	"fmt"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
	"time"
)

var releaseTag = regexp.MustCompile(`^v(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)$`)
var numericVersion = regexp.MustCompile(`^(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)$`)
var commitHash = regexp.MustCompile(`^[a-f0-9]{40,64}$`)
var displayVersion = regexp.MustCompile(`^(dev|v[0-9]+\.[0-9]+\.[0-9]+)(-[a-f0-9]{8})?$`)

type versionInfo struct {
	Version        string `json:"version"`
	PackageVersion string `json:"package-version"`
	Commit         string `json:"commit"`
	Release        bool   `json:"release"`
}

func git(dir string, args ...string) (string, error) {
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	data, err := cmd.Output()
	return strings.TrimSpace(string(data)), err
}

func resolveVersion(dir string, getenv func(string) string, fallback string) (versionInfo, error) {
	isCI := getenv("GITHUB_ACTIONS") == "true" || getenv("CI") == "true"
	result := versionInfo{Version: "dev", PackageVersion: fallback}
	if _, err := parsePackageVersion(result.PackageVersion); err != nil {
		return result, err
	}
	result.Commit, _ = git(dir, "rev-parse", "HEAD")
	if !commitHash.MatchString(result.Commit) {
		result.Commit = ""
	}
	if isCI && result.Commit != "" {
		result.Version += "-" + result.Commit[:8]
		tags, err := git(dir, "for-each-ref", "--format=%(refname:short)", "refs/tags")
		if err != nil {
			return result, fmt.Errorf("read Git tags: %w", err)
		}
		args := []string{"describe", "--tags", "--abbrev=0"}
		for tag := range strings.FieldsSeq(tags) {
			if releaseTag.MatchString(tag) {
				args = append(args, "--match", tag)
			}
		}
		if len(args) > 3 {
			if tag, err := git(dir, args...); err == nil {
				result.Version = tag + "-" + result.Commit[:8]
				result.PackageVersion = strings.TrimPrefix(tag, "v")
			}
		}
	}
	// A tag mentioned by a branch/manual/PR event is never a release trigger.
	if getenv("GITHUB_ACTIONS") == "true" && getenv("GITHUB_EVENT_NAME") == "push" && getenv("GITHUB_REF_TYPE") == "tag" {
		tag := getenv("GITHUB_REF_NAME")
		if releaseTag.MatchString(tag) {
			tagCommit, err := git(dir, "rev-parse", tag+"^{commit}")
			if err != nil || result.Commit == "" || tagCommit != result.Commit {
				return result, fmt.Errorf("release tag %q does not resolve to Git HEAD", tag)
			}
			result.Version = tag
			result.PackageVersion = strings.TrimPrefix(tag, "v")
			result.Release = true
		}
	}
	if override := getenv("DAN_TICK_VERSION"); override != "" {
		if !displayVersion.MatchString(override) {
			return result, fmt.Errorf("invalid DAN_TICK_VERSION %q", override)
		}
		result.Version = override
	}
	if override := getenv("DAN_TICK_COMMIT"); override != "" {
		if !commitHash.MatchString(override) {
			return result, fmt.Errorf("invalid DAN_TICK_COMMIT %q", override)
		}
		result.Commit = override
	}
	if override := getenv("DAN_TICK_PACKAGE_VERSION"); override != "" {
		result.PackageVersion = override
	} else if override := getenv("DAN_TICK_VERSION"); override != "" && strings.HasPrefix(override, "v") {
		result.PackageVersion = strings.SplitN(strings.TrimPrefix(override, "v"), "-", 2)[0]
	}
	_, err := parsePackageVersion(result.PackageVersion)
	return result, err
}

func parsePackageVersion(version string) (string, error) {
	if !numericVersion.MatchString(version) {
		return "", fmt.Errorf("package version must contain three numeric components: %q", version)
	}
	for component := range strings.SplitSeq(version, ".") {
		value, err := strconv.ParseUint(component, 10, 16)
		if err != nil || value > 65535 {
			return "", fmt.Errorf("package version component exceeds Windows limit 65535: %q", version)
		}
	}
	return version + ".0", nil
}

func metadataFlags(info versionInfo, compiledAt time.Time) string {
	const prefix = "github.com/dongwlin/dan-tick/internal/buildinfo."
	return "-X " + prefix + "Version=" + info.Version +
		" -X " + prefix + "BuildTime=" + compiledAt.UTC().Format(time.RFC3339) +
		" -X " + prefix + "Commit=" + info.Commit
}
