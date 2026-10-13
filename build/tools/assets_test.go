package main

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

func TestAssetsUseConfigAndSeparateDisplayVersion(t *testing.T) {
	buildDir := t.TempDir()
	configPath := filepath.Join(buildDir, "config.yml")
	configData := `version: '3'
info:
  companyName: Example Company
  productName: Example App
  productIdentifier: io.example.app
  description: Example description
  copyright: Example copyright
  comments: ""
  version: 9.9.9
fileAssociations:
  - ext: example
    name: Example
protocols:
  - scheme: example
    description: Example protocol
`
	require.NoError(t, os.WriteFile(configPath, []byte(configData), 0644))
	config, err := readConfig(configPath)
	require.NoError(t, err)
	custom := filepath.Join(buildDir, "darwin", "Info.plist")
	require.NoError(t, writeAsset(custom, []byte("custom platform resource")))
	installer := filepath.Join(buildDir, "windows", "nsis", "project.nsi")
	require.NoError(t, writeAsset(installer, []byte("custom installer")))
	info := versionInfo{Version: "v0.1.0-abcdef12", PackageVersion: "0.1.0"}
	generate := func(args []string) error {
		option := func(key string) string {
			for i := range args {
				if args[i] == key {
					return args[i+1]
				}
			}
			t.Fatalf("missing option %s", key)
			return ""
		}
		require.Equal(t, config.Info.CompanyName, option("-productcompany"))
		require.Empty(t, option("-productcomments"))
		data, err := os.ReadFile(option("-config"))
		require.NoError(t, err)
		var preserved map[string]any
		require.NoError(t, yaml.Unmarshal(data, &preserved))
		require.Equal(t, "0.1.0", preserved["info"].(map[string]any)["version"])
		require.NotEmpty(t, preserved["fileAssociations"])
		require.NotEmpty(t, preserved["protocols"])
		dir := option("-dir")
		generated := map[string]string{
			"windows/info.json":            `{"fixed":{"file_version":"0.1.0"},"info":{"0000":{"ProductVersion":"0.1.0","CompanyName":"Example Company","Comments":""}}}`,
			"windows/wails.exe.manifest":   `<assembly><assemblyIdentity name="io.example.app" version="0.1.0"/></assembly>`,
			"windows/nsis/wails_tools.nsh": "!define INFO_PRODUCTVERSION \"0.1.0\"\n!define WAILS_INSTALL_SCOPE \"machine\"\nWriteRegStr HKCU \"${UNINST_KEY}\" \"DisplayVersion\" \"${INFO_PRODUCTVERSION}\"\n",
			"darwin/Info.plist":            "upstream generated resource",
		}
		for path, content := range generated {
			require.NoError(t, writeAsset(filepath.Join(dir, filepath.FromSlash(path)), []byte(content)))
		}
		return nil
	}
	require.NoError(t, updateAssets(configPath, config, info, "all", "example", generate))
	data, err := os.ReadFile(filepath.Join(buildDir, "windows", "info.json"))
	require.NoError(t, err)
	var resource map[string]any
	require.NoError(t, json.Unmarshal(data, &resource))
	require.Equal(t, "0.1.0.0", resource["fixed"].(map[string]any)["file_version"])
	require.Equal(t, "0.1.0.0", resource["fixed"].(map[string]any)["product_version"])
	require.Equal(t, info.Version, resource["info"].(map[string]any)["0000"].(map[string]any)["ProductVersion"])
	require.Equal(t, info.Version, resource["info"].(map[string]any)["0000"].(map[string]any)["FileVersion"])
	data, err = os.ReadFile(filepath.Join(buildDir, "windows", "wails.exe.manifest"))
	require.NoError(t, err)
	require.Contains(t, string(data), `version="0.1.0.0"`)
	data, err = os.ReadFile(filepath.Join(buildDir, "windows", "nsis", "wails_tools.nsh"))
	require.NoError(t, err)
	require.Contains(t, string(data), `!define WAILS_INSTALL_SCOPE "user"`)
	require.Contains(t, string(data), `"DisplayVersion" "${INFO_DISPLAYVERSION}"`)
	require.Contains(t, string(data), `!define INFO_DISPLAYVERSION "v0.1.0-abcdef12"`)
	data, err = os.ReadFile(filepath.Join(buildDir, "linux", "example.desktop"))
	require.NoError(t, err)
	require.Contains(t, string(data), "Name=Example App\n")
	require.Contains(t, string(data), "Comment=Example description\n")
	require.Contains(t, string(data), "Categories=Utility;\n")
	for path, expected := range map[string]string{custom: "custom platform resource", installer: "custom installer", configPath: configData} {
		data, err := os.ReadFile(path)
		require.NoError(t, err)
		require.Equal(t, expected, string(data))
	}
}

func TestDesktopEscapesConfigFields(t *testing.T) {
	require.Equal(t, `line\nnext\\path\t`, desktopString("line\nnext\\path\t"))
}

func TestInvalidResourceConfigFails(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yml")
	require.NoError(t, os.WriteFile(path, []byte("version: '3'\n"), 0644))
	_, err := readConfig(path)
	require.ErrorContains(t, err, "must define info")
	_, err = resolveVersion(t.TempDir(), env(map[string]string{"DAN_TICK_VERSION": "bad\nversion", "DAN_TICK_COMMIT": strings.Repeat("a", 40)}), "0.0.0")
	require.ErrorContains(t, err, "invalid DAN_TICK_VERSION")
}

func TestWailsWindowsAssetsCompatibility(t *testing.T) {
	if _, err := exec.LookPath("wails3"); err != nil {
		t.Skip("Wails CLI is not installed")
	}
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yml")
	require.NoError(t, os.WriteFile(path, []byte(`version: '3'
info:
  companyName: Example
  productName: Example App
  productIdentifier: io.example.app
  description: Example description
  copyright: Example copyright
  comments: ""
  version: 9.9.9
fileAssociations:
  - ext: example
    name: Example
    description: Example file
    iconName: example
    role: Editor
`), 0644))
	config, err := readConfig(path)
	require.NoError(t, err)
	info := versionInfo{Version: "v0.1.0-abcdef12", PackageVersion: "0.1.0"}
	require.NoError(t, updateAssets(path, config, info, "windows", "example", runWails))
	data, err := os.ReadFile(filepath.Join(dir, "windows", "info.json"))
	require.NoError(t, err)
	require.Contains(t, string(data), `"file_version": "0.1.0.0"`)
	require.Contains(t, string(data), `"product_version": "0.1.0.0"`)
	require.Contains(t, string(data), `"ProductVersion": "v0.1.0-abcdef12"`)
	require.Contains(t, string(data), `"FileVersion": "v0.1.0-abcdef12"`)
	require.Contains(t, string(data), `"Comments": ""`)
	data, err = os.ReadFile(filepath.Join(dir, "windows", "nsis", "wails_tools.nsh"))
	require.NoError(t, err)
	require.Contains(t, string(data), `!define INFO_DISPLAYVERSION "v0.1.0-abcdef12"`)
	require.Contains(t, string(data), `!define WAILS_INSTALL_SCOPE "user"`)
	require.Contains(t, string(data), `"DisplayVersion" "${INFO_DISPLAYVERSION}"`)
	require.Contains(t, string(data), `APP_ASSOCIATE "example"`)
}
