package main

import (
	"cmp"
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/adrg/xdg"
	"gopkg.in/yaml.v3"
)

// ConfigFile .
type ConfigFile struct {
	Presets map[string]ConfigPreset `yaml:"presets"`
}

// ConfigPreset .
type ConfigPreset struct {
	Values []ConfigPresetValue `yaml:"values"`
}

// ConfigPresetValue .
type ConfigPresetValue struct {
	ID    ControlID `yaml:"id"`
	Value int32     `yaml:"value"`
}

func LoadConfigFile() (ConfigFile, error) {
	configFilePath, err := xdg.ConfigFile("webcam/config.yml")
	if err != nil {
		return ConfigFile{}, fmt.Errorf("can't get config file path: %w", err)
	}

	configDir := filepath.Dir(configFilePath)
	if err := os.MkdirAll(configDir, 0700); err != nil {
		return ConfigFile{}, fmt.Errorf("could not create directory for config file: %w", err)
	}

	var config ConfigFile

	configData, err := os.ReadFile(configFilePath)
	if err == nil {
		if err := yaml.Unmarshal(configData, &config); err != nil {
			return ConfigFile{}, fmt.Errorf("error parsing config gile: %w", err)
		}
	}
	if config.Presets == nil {
		config.Presets = make(map[string]ConfigPreset)
	}
	return config, nil

}

func SaveConfigFile(config ConfigFile) error {
	configFilePath, err := xdg.ConfigFile("webcam/config.yml")
	if err != nil {
		return fmt.Errorf("can't get config file path: %w", err)
	}

	data, err := yaml.Marshal(&config)
	if err != nil {
		return fmt.Errorf("could not render config file: %w", err)
	}
	if err := os.WriteFile(configFilePath, data, 0700); err != nil {
		return fmt.Errorf("could not write confi gile: %w", err)
	}
	return nil
}

func newPreset(wc *Webcam) (ConfigPreset, error) {
	var values []ConfigPresetValue

	controls := wc.getControls()
	for _, c := range controls {
		values = append(values, ConfigPresetValue{
			ID:    c.ID,
			Value: c.Value,
		})

	}
	return ConfigPreset{
		Values: values,
	}, nil
}

// applyPreset sets the camera's controls to the values in preset.
//
// Some controls only accept writes while a related automatic mode is off. For
// example, White Balance Temperature is inactive while White Balance,
// Automatic is on. The driver reports this with V4L2_CTRL_FLAG_INACTIVE. So
// applyPreset works in passes: each pass reads the current flags, sets the
// boolean and menu controls (which hold the automatic modes) before the
// integer controls, and skips controls that are inactive or read-only. Passes
// repeat while the previous one set something, because a change can make
// other controls active. Inactive controls are left alone: their value is
// driven by the automatic mode. Finally every control is read back, since
// some cameras silently drop writes.
//
// Every control is written once even if it already reports the preset's
// value. Cameras do not always report what they use: some keep returning the
// last manual exposure time while auto exposure runs, so after switching back
// to manual the reported value can match the preset while the camera uses
// whatever exposure the automatic mode last chose.
func applyPreset(wc *Webcam, preset ConfigPreset) error {
	want := make(map[ControlID]int32, len(preset.Values))
	for _, v := range preset.Values {
		want[v.ID] = v.Value
	}

	set := make(map[ControlID]bool, len(want))
	failed := make(map[ControlID]error)
	// Each pass but the last sets at least one more control, so this ends.
	for progress := true; progress; {
		progress = false
		clear(failed)
		controls := wc.getControls()
		// Stable so that controls keep ID order within each group.
		slices.SortStableFunc(controls, func(a, b Control) int {
			return cmp.Compare(controlSetOrder(a), controlSetOrder(b))
		})
		for _, c := range controls {
			value, ok := want[c.ID]
			if !ok || set[c.ID] || c.IsReadOnly() || c.IsInactive() {
				continue
			}
			if err := wc.SetControl(c.ID, value); err != nil {
				failed[c.ID] = fmt.Errorf("%s: %w", c.Name, err)
				continue
			}
			set[c.ID] = true
			progress = true
		}
	}

	// Some cameras accept a write without error and then drop the value, so
	// read everything back.
	for _, c := range wc.getControls() {
		value, ok := want[c.ID]
		if !ok || c.Value == value || c.IsReadOnly() || c.IsInactive() {
			continue
		}
		if _, ok := failed[c.ID]; !ok {
			failed[c.ID] = fmt.Errorf("%s: is %d after setting %d", c.Name, c.Value, value)
		}
	}

	if len(failed) == 0 {
		return nil
	}
	var msgs []string
	for _, id := range slices.Sorted(maps.Keys(failed)) {
		msgs = append(msgs, failed[id].Error())
	}
	return fmt.Errorf("could not set %s", strings.Join(msgs, "; "))
}

// controlSetOrder returns 0 for controls that applyPreset sets first.
func controlSetOrder(c Control) int {
	if c.Type == c_bool || c.Type == c_menu {
		return 0
	}
	return 1
}
