package main

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

type buildConfig struct {
	Info struct {
		CompanyName       string `yaml:"companyName"`
		ProductName       string `yaml:"productName"`
		ProductIdentifier string `yaml:"productIdentifier"`
		Description       string `yaml:"description"`
		Copyright         string `yaml:"copyright"`
		Comments          string `yaml:"comments"`
		Version           string `yaml:"version"`
	} `yaml:"info"`
	data []byte
}

func readConfig(path string) (buildConfig, error) {
	var config buildConfig
	data, err := os.ReadFile(path)
	if err != nil {
		return config, err
	}
	if err := yaml.Unmarshal(data, &config); err != nil {
		return config, err
	}
	if config.Info.ProductName == "" || config.Info.ProductIdentifier == "" || config.Info.Version == "" {
		return config, fmt.Errorf("%s must define info.productName, info.productIdentifier, and info.version", path)
	}
	config.data = data
	return config, nil
}

func runWails(args []string) error {
	cmd := exec.Command("wails3", args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

func updateAssets(configPath string, config buildConfig, info versionInfo, platform, name string, generate func([]string) error) error {
	if platform != "all" && platform != "windows" && platform != "linux" {
		return fmt.Errorf("unsupported resource platform %q", platform)
	}
	if name == "" || strings.ContainsAny(name, "\\/\r\n\"$") {
		return fmt.Errorf("invalid binary name %q", name)
	}
	buildDir := filepath.Dir(configPath)
	if platform == "all" || platform == "windows" {
		if err := updateWindowsAssets(buildDir, config, info, name, generate); err != nil {
			return err
		}
	}
	if platform == "all" || platform == "linux" {
		desktop := "[Desktop Entry]\nType=Application\nName=" + desktopString(config.Info.ProductName) +
			"\nComment=" + desktopString(config.Info.Description) + "\nExec=" + name + "\nIcon=" + name +
			"\nTerminal=false\nCategories=Utility;\nStartupWMClass=" + desktopString(config.Info.ProductIdentifier) + "\n"
		return writeAsset(filepath.Join(buildDir, "linux", name+".desktop"), []byte(desktop))
	}
	return nil
}

func updateWindowsAssets(buildDir string, config buildConfig, info versionInfo, name string, generate func([]string) error) error {
	fullVersion, err := parsePackageVersion(info.PackageVersion)
	if err != nil {
		return err
	}
	tempDir, err := os.MkdirTemp("", "dan-tick-build-assets-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(tempDir)
	// Preserve the complete YAML, including file associations and protocols.
	// Wails treats its default productversion as unset, so replace the temporary
	// config version too; a v0.1.0 tag must not pick up a stale config fallback.
	var document map[string]any
	if err := yaml.Unmarshal(config.data, &document); err != nil {
		return err
	}
	document["info"].(map[string]any)["version"] = info.PackageVersion
	data, err := yaml.Marshal(document)
	if err != nil {
		return err
	}
	tempConfig := filepath.Join(tempDir, "config.yml")
	if err := os.WriteFile(tempConfig, data, 0600); err != nil {
		return err
	}
	args := []string{"update", "build-assets", "-name", name, "-binaryname", name, "-config", tempConfig, "-dir", tempDir,
		"-productversion", info.PackageVersion, "-productcompany", config.Info.CompanyName, "-productname", config.Info.ProductName,
		"-productidentifier", config.Info.ProductIdentifier, "-productdescription", config.Info.Description,
		"-productcopyright", config.Info.Copyright, "-productcomments", config.Info.Comments}
	if err := generate(args); err != nil {
		return fmt.Errorf("generate Windows resources: %w", err)
	}
	jsonPath := filepath.Join(tempDir, "windows", "info.json")
	data, err = os.ReadFile(jsonPath)
	if err != nil {
		return err
	}
	var resource map[string]any
	if err := json.Unmarshal(data, &resource); err != nil {
		return err
	}
	resource["fixed"].(map[string]any)["file_version"] = fullVersion
	resource["fixed"].(map[string]any)["product_version"] = fullVersion
	resource["info"].(map[string]any)["0000"].(map[string]any)["ProductVersion"] = info.Version
	resource["info"].(map[string]any)["0000"].(map[string]any)["FileVersion"] = info.Version
	data, err = json.MarshalIndent(resource, "", "\t")
	if err != nil {
		return err
	}
	if err := writeAsset(filepath.Join(buildDir, "windows", "info.json"), append(data, '\n')); err != nil {
		return err
	}
	for _, relative := range []string{"windows/wails.exe.manifest", "windows/nsis/wails_tools.nsh"} {
		data, err := os.ReadFile(filepath.Join(tempDir, filepath.FromSlash(relative)))
		if err != nil {
			return err
		}
		if strings.HasSuffix(relative, ".manifest") {
			data = []byte(strings.Replace(string(data), `version="`+info.PackageVersion+`"`, `version="`+fullVersion+`"`, 1))
		} else {
			text := strings.Replace(string(data), `!define WAILS_INSTALL_SCOPE "machine"`, `!define WAILS_INSTALL_SCOPE "user"`, 1)
			text = strings.ReplaceAll(text, `"DisplayVersion" "${INFO_PRODUCTVERSION}"`, `"DisplayVersion" "${INFO_DISPLAYVERSION}"`)
			text = "!ifndef INFO_DISPLAYVERSION\n    !define INFO_DISPLAYVERSION \"" + info.Version + "\"\n!endif\n\n" + text
			text = strings.TrimRight(text, "\n") + "\n"
			data = []byte(text)
		}
		if err := writeAsset(filepath.Join(buildDir, filepath.FromSlash(relative)), data); err != nil {
			return err
		}
	}
	return nil
}

func writeAsset(path string, data []byte) error {
	if previous, err := os.ReadFile(path); err == nil && string(previous) == string(data) {
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return err
	}
	return os.WriteFile(path, data, 0644)
}

func desktopString(value string) string {
	return strings.NewReplacer("\\", "\\\\", "\n", "\\n", "\r", "\\r", "\t", "\\t").Replace(value)
}
