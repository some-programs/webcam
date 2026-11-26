package main

import (
	"cmp"
	"errors"
	"fmt"
	"io/fs"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/adrg/xdg"
	"github.com/pelletier/go-toml/v2"
)

// ConfigFile is the config file, config.toml.
type ConfigFile struct {
	// Cameras holds the settings of each camera model, keyed by the name
	// its driver reports, such as "C922 Pro Stream Webcam".
	Cameras map[string]*CameraConfig `toml:"cameras,omitempty"`

	// path is the file the config was loaded from.
	path string
}

// CameraConfig holds the settings of one camera model.
type CameraConfig struct {
	// Presets are keyed by the function key they are on, "F1" to "F12".
	Presets map[string]ConfigPreset `toml:"presets"`
}

// ConfigPreset maps control names, such as "Brightness", to values.
type ConfigPreset map[string]int32

// CameraPresets returns the presets of camera, which can be changed in place.
func (c *ConfigFile) CameraPresets(camera string) map[string]ConfigPreset {
	if c.Cameras == nil {
		c.Cameras = make(map[string]*CameraConfig)
	}
	cc, ok := c.Cameras[camera]
	if !ok {
		cc = &CameraConfig{}
		c.Cameras[camera] = cc
	}
	if cc.Presets == nil {
		cc.Presets = make(map[string]ConfigPreset)
	}
	return cc.Presets
}

// defaultConfigPath returns $XDG_CONFIG_HOME/webcam/config.toml, creating
// its directory if needed.
func defaultConfigPath() (string, error) {
	path, err := xdg.ConfigFile("webcam/config.toml")
	if err != nil {
		return "", fmt.Errorf("can't get config file path: %w", err)
	}
	return path, nil
}

// LoadConfigFile reads the config file at path, or at the default path if
// path is empty. A missing file at the default path gives an empty config.
func LoadConfigFile(path string) (ConfigFile, error) {
	explicit := path != ""
	if !explicit {
		var err error
		if path, err = defaultConfigPath(); err != nil {
			return ConfigFile{}, err
		}
	}

	return readConfigFile(path, !explicit)
}

// readConfigFile reads the config file at path. If missingOK is set, a
// missing file gives an empty config.
func readConfigFile(path string, missingOK bool) (ConfigFile, error) {
	config := ConfigFile{path: path}
	data, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) && missingOK {
		return config, nil
	}
	if err != nil {
		return ConfigFile{}, fmt.Errorf("could not read config file: %w", err)
	}
	if err := toml.Unmarshal(data, &config); err != nil {
		return ConfigFile{}, fmt.Errorf("error parsing config file %s: %w", path, err)
	}
	return config, nil
}

// ReloadConfigFile reads the file config was loaded from again, to pick up
// changes made by other programs. A missing file gives an empty config.
func ReloadConfigFile(config ConfigFile) (ConfigFile, error) {
	return readConfigFile(config.path, true)
}

// SavePreset stores preset as name for camera in config's file and returns
// the config as saved. It reads the file again first, so that presets that
// other programs saved since config was loaded are kept.
func SavePreset(config ConfigFile, camera, name string, preset ConfigPreset) (ConfigFile, error) {
	fresh, err := ReloadConfigFile(config)
	if err != nil {
		return config, err
	}
	fresh.CameraPresets(camera)[name] = preset
	if err := SaveConfigFile(fresh); err != nil {
		return config, err
	}
	return fresh, nil
}

// SaveConfigFile writes config to the file it was loaded from. It writes a
// new file and renames it over the old one, so that programs reading the
// file never see it half written.
func SaveConfigFile(config ConfigFile) error {
	data, err := toml.Marshal(&config)
	if err != nil {
		return fmt.Errorf("could not render config file: %w", err)
	}
	// Replace the file a symlink points to, not the symlink.
	path, err := filepath.EvalSymlinks(config.path)
	if errors.Is(err, fs.ErrNotExist) {
		path = config.path
	} else if err != nil {
		return fmt.Errorf("could not write config file: %w", err)
	}
	f, err := os.CreateTemp(filepath.Dir(path), ".config-*.toml")
	if err != nil {
		return fmt.Errorf("could not write config file: %w", err)
	}
	defer os.Remove(f.Name()) // fails harmlessly once renamed
	if _, err := f.Write(data); err != nil {
		f.Close()
		return fmt.Errorf("could not write config file: %w", err)
	}
	if err := f.Close(); err != nil {
		return fmt.Errorf("could not write config file: %w", err)
	}
	if err := os.Rename(f.Name(), path); err != nil {
		return fmt.Errorf("could not write config file: %w", err)
	}
	return nil
}

// newPreset returns a preset with the current value of every control.
func newPreset(wc *Webcam) (ConfigPreset, error) {
	p := make(ConfigPreset)
	for _, c := range wc.getControls() {
		p[c.Name] = c.Value
	}
	return p, nil
}

// defaultPreset returns a preset that sets every control to its default
// value.
func defaultPreset(wc *Webcam) ConfigPreset {
	p := make(ConfigPreset)
	for _, c := range wc.getControls() {
		p[c.Name] = c.Default
	}
	return p
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
//
// Controls are matched by name. Names in preset that the camera has no
// control for are reported in the error, after the other controls are set.
func applyPreset(wc *Webcam, preset ConfigPreset) error {
	want := make(map[ControlID]int32, len(preset))
	unknown := maps.Clone(preset)
	for _, c := range wc.getControls() {
		if value, ok := preset[c.Name]; ok {
			want[c.ID] = value
			delete(unknown, c.Name)
		}
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

	var msgs []string
	for _, id := range slices.Sorted(maps.Keys(failed)) {
		msgs = append(msgs, failed[id].Error())
	}
	for _, name := range slices.Sorted(maps.Keys(unknown)) {
		msgs = append(msgs, fmt.Sprintf("%s: no such control", name))
	}
	if len(msgs) == 0 {
		return nil
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
