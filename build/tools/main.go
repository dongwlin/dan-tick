// Command tools supplies the shared version and resource inputs for desktop builds.
package main

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"
)

func main() {
	if err := run(os.Args[1:], os.Stdout); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run(args []string, out io.Writer) error {
	if len(args) == 0 {
		return errors.New("usage: tools version|ldflags|assets|prepare-dist")
	}
	flags := flag.NewFlagSet(args[0], flag.ContinueOnError)
	configPath := flags.String("config", "build/config.yml", "Wails configuration")
	githubOutput := flags.String("github-output", "", "append version outputs for GitHub Actions")
	platform := flags.String("platform", "all", "resource platform: windows, linux, or all")
	name := flags.String("name", "dan-tick", "binary name")
	if err := flags.Parse(args[1:]); err != nil {
		return err
	}
	if flags.NArg() != 0 {
		return fmt.Errorf("unexpected arguments: %v", flags.Args())
	}
	if args[0] == "prepare-dist" {
		return prepareDist("frontend/dist/index.html")
	}
	config, err := readConfig(*configPath)
	if err != nil {
		return err
	}
	info, err := resolveVersion(".", os.Getenv, config.Info.Version)
	if err != nil {
		return err
	}
	switch args[0] {
	case "version":
		if *githubOutput == "" {
			return json.NewEncoder(out).Encode(info)
		}
		file, err := os.OpenFile(*githubOutput, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0600)
		if err != nil {
			return err
		}
		defer file.Close()
		_, err = fmt.Fprintf(file, "version=%s\npackage-version=%s\ncommit=%s\nrelease=%t\n", info.Version, info.PackageVersion, info.Commit, info.Release)
		return err
	case "ldflags":
		_, err := fmt.Fprintln(out, metadataFlags(info, time.Now()))
		return err
	case "assets":
		return updateAssets(*configPath, config, info, *platform, *name, runWails)
	default:
		return fmt.Errorf("unknown command %q", args[0])
	}
}

func prepareDist(path string) error {
	if _, err := os.Stat(path); err == nil {
		return nil
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return err
	}
	// Binding generation scans main.go before Vite has produced its first build.
	return os.WriteFile(path, []byte("<!doctype html><html><body></body></html>\n"), 0644)
}
