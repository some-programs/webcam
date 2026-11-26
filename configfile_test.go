package main

import (
	"maps"
	"os"
	"path/filepath"
	"testing"
)

func TestConfigFileRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.toml")
	config := ConfigFile{path: path}
	config.CameraPresets("C922 Pro Stream Webcam")["F1"] = ConfigPreset{
		"Brightness":               128,
		"White Balance, Automatic": 0,
		"Pan, Absolute":            -3600,
	}
	if err := SaveConfigFile(config); err != nil {
		t.Fatal(err)
	}

	loaded, err := LoadConfigFile(path)
	if err != nil {
		t.Fatal(err)
	}
	got := loaded.CameraPresets("C922 Pro Stream Webcam")["F1"]
	want := config.CameraPresets("C922 Pro Stream Webcam")["F1"]
	if !maps.Equal(got, want) {
		t.Errorf("loaded preset = %v, want %v", got, want)
	}
	if other := loaded.CameraPresets("Other Camera"); len(other) != 0 {
		t.Errorf("Other Camera presets = %v, want none", other)
	}
}

// TestSavePresetKeepsOtherInstancesPresets checks that two programs that
// loaded the config at the same time don't drop each other's presets.
func TestSavePresetKeepsOtherInstancesPresets(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.toml")
	if err := SaveConfigFile(ConfigFile{path: path}); err != nil {
		t.Fatal(err)
	}
	a, err := LoadConfigFile(path)
	if err != nil {
		t.Fatal(err)
	}
	b, err := LoadConfigFile(path)
	if err != nil {
		t.Fatal(err)
	}

	const camera = "C922 Pro Stream Webcam"
	if _, err := SavePreset(a, camera, "F1", ConfigPreset{"Brightness": 1}); err != nil {
		t.Fatal(err)
	}
	b, err = SavePreset(b, camera, "F2", ConfigPreset{"Brightness": 2})
	if err != nil {
		t.Fatal(err)
	}
	if got := len(b.CameraPresets(camera)); got != 2 {
		t.Errorf("returned config has %d presets, want 2", got)
	}

	loaded, err := LoadConfigFile(path)
	if err != nil {
		t.Fatal(err)
	}
	presets := loaded.CameraPresets(camera)
	if presets["F1"]["Brightness"] != 1 || presets["F2"]["Brightness"] != 2 {
		t.Errorf("saved presets = %v, want F1 and F2", presets)
	}
	matches, _ := filepath.Glob(filepath.Join(filepath.Dir(path), ".config-*"))
	if len(matches) > 0 {
		t.Errorf("temporary files left behind: %v", matches)
	}
}

func TestSaveConfigFileKeepsSymlink(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "real.toml")
	link := filepath.Join(dir, "config.toml")
	if err := os.WriteFile(target, nil, 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, link); err != nil {
		t.Fatal(err)
	}
	config, err := LoadConfigFile(link)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := SavePreset(config, "cam", "F1", ConfigPreset{"Brightness": 1}); err != nil {
		t.Fatal(err)
	}
	if fi, err := os.Lstat(link); err != nil || fi.Mode()&os.ModeSymlink == 0 {
		t.Fatalf("%s is no longer a symlink (err %v)", link, err)
	}
	loaded, err := LoadConfigFile(target)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.CameraPresets("cam")["F1"]["Brightness"] != 1 {
		t.Errorf("preset not written to the symlink's target")
	}
}
