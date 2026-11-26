package main

import (
	"maps"
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
