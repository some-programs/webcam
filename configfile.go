package main

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

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

func applyPreset(wc *Webcam, preset ConfigPreset) error {
	presetValuesMap := make(map[ControlID]ConfigPresetValue)
	for _, v := range preset.Values {
		presetValuesMap[v.ID] = v
	}

	allControls := wc.getControls()
	var boolAndMenuControls []Control
	var integerControls []Control
	for _, c := range allControls {
		if c.Type == c_menu || c.Type == c_bool {
			boolAndMenuControls = append(boolAndMenuControls, c)
		} else {
			integerControls = append(integerControls, c)
		}
	}

	// my webcam is not correctly reporting locked controls so we try to set
	// all controls a number of times hoping that whatever controls are locking
	// other controls are resolved during the rety process.
	for _, controls := range [][]Control{boolAndMenuControls, integerControls} {
		todo := make(map[ControlID]any)
		for _, c := range controls {
			todo[c.ID] = struct{}{}
		}
		var retries int
		for len(todo) > 0 && (retries < len(todo)*10) {
			// rand.Shuffle(len(controls), func(i, j int) {
			// 	controls[i], controls[j] = controls[j], controls[i]
			// })
			for _, v := range controls {
				if _, ok := todo[v.ID]; !ok {
					continue
				}
				if err := wc.webcam.SetControl(v.ID, presetValuesMap[v.ID].Value); err != nil {
					time.Sleep(25 * time.Millisecond)
					retries++
					continue
				}
				time.Sleep(5 * time.Millisecond)
				delete(todo, v.ID)
			}
		}
	}

	return nil
}
