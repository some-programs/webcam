package main

import (
	"testing"

	"gopkg.in/yaml.v3"
)

func TestCameraPresetsMovesLegacyPresets(t *testing.T) {
	var config ConfigFile
	data := []byte(`
presets:
    "1":
        values:
            - id: 9963776
              value: 128
`)
	if err := yaml.Unmarshal(data, &config); err != nil {
		t.Fatal(err)
	}

	presets := config.CameraPresets("cam A")
	if got := presets["1"].Values; len(got) != 1 || got[0].Value != 128 {
		t.Fatalf("cam A presets = %v, want the legacy preset", presets)
	}
	if config.LegacyPresets != nil {
		t.Errorf("LegacyPresets = %v after moving them, want nil", config.LegacyPresets)
	}
	if other := config.CameraPresets("cam B"); len(other) != 0 {
		t.Errorf("cam B presets = %v, want none", other)
	}

	// Presets added through the returned map are saved.
	presets["2"] = ConfigPreset{}
	out, err := yaml.Marshal(&config)
	if err != nil {
		t.Fatal(err)
	}
	var reloaded ConfigFile
	if err := yaml.Unmarshal(out, &reloaded); err != nil {
		t.Fatal(err)
	}
	if got := len(reloaded.CameraPresets("cam A")); got != 2 {
		t.Errorf("cam A has %d presets after saving and loading, want 2\n%s", got, out)
	}
	if reloaded.LegacyPresets != nil {
		t.Errorf("saved config still has legacy presets:\n%s", out)
	}
}
