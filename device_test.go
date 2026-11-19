package main

// Tests in this file run against a real V4L2 camera. They are skipped unless
// TEST_WEBCAM_DEVICE names the device to use:
//
//	TEST_WEBCAM_DEVICE=/dev/video0 go test -v -run Device ./...
//
// The tests change the camera's controls. Each test saves all control values
// first and restores them when it finishes, but the picture of anything
// currently using the camera will change while the tests run.

import (
	"os"
	"slices"
	"testing"
	"unsafe"
)

const testDeviceEnv = "TEST_WEBCAM_DEVICE"

// Control IDs from linux/v4l2-controls.h.
const (
	cidHue                     ControlID = 0x00980903
	cidAutoWhiteBalance        ControlID = 0x0098090c
	cidHueAuto                 ControlID = 0x00980919
	cidWhiteBalanceTemperature ControlID = 0x0098091a
	cidExposureAuto            ControlID = 0x009a0901
	cidExposureAbsolute        ControlID = 0x009a0902
	cidFocusAbsolute           ControlID = 0x009a090a
	cidFocusAuto               ControlID = 0x009a090c
)

// Values of V4L2_CID_EXPOSURE_AUTO (enum v4l2_exposure_auto_type).
const (
	exposureAuto             = 0
	exposureManual           = 1
	exposureAperturePriority = 3
)

// autoPair is an automatic-mode control and the control whose value it
// drives while the automatic mode is on.
type autoPair struct {
	name   string
	auto   ControlID
	manual ControlID
	// off is the value of auto that turns the automatic mode off.
	off int32
	// on lists values of auto that turn the automatic mode on, in order of
	// preference. Empty means any valid value other than off.
	on []int32
}

var autoPairs = []autoPair{
	{name: "white balance", auto: cidAutoWhiteBalance, manual: cidWhiteBalanceTemperature, off: 0},
	{name: "hue", auto: cidHueAuto, manual: cidHue, off: 0},
	{name: "focus", auto: cidFocusAuto, manual: cidFocusAbsolute, off: 0},
	// Shutter priority is left out on purpose: in that mode the exposure
	// time is set by the user, yet some drivers still flag it inactive.
	{name: "exposure", auto: cidExposureAuto, manual: cidExposureAbsolute, off: exposureManual,
		on: []int32{exposureAperturePriority, exposureAuto}},
}

// struct v4l2_querymenu
type v4l2QueryMenu struct {
	ID       uint32
	Index    uint32
	Name     [32]byte // union with int64 value; the C struct is packed
	Reserved uint32
}

var vidiocQueryMenu = ioc(iocReadWrite, 37, unsafe.Sizeof(v4l2QueryMenu{}))

// menuValues returns the valid values of menu control c. Menus may have gaps
// between Min and Max.
func menuValues(t *testing.T, wc *Webcam, c Control) []int32 {
	t.Helper()
	var values []int32
	for i := c.Min; i <= c.Max; i++ {
		q := v4l2QueryMenu{ID: c.ID, Index: uint32(i)}
		if err := ioctl(wc.fd, vidiocQueryMenu, unsafe.Pointer(&q)); err == nil {
			values = append(values, i)
		}
	}
	if len(values) == 0 {
		t.Fatalf("%s: menu has no valid items in [%d, %d]", c.Name, c.Min, c.Max)
	}
	return values
}

// openTestCam opens the device named by TEST_WEBCAM_DEVICE, or skips the
// test if it is unset. The camera's control values are restored when the
// test finishes.
func openTestCam(t *testing.T) *Webcam {
	t.Helper()
	dev := os.Getenv(testDeviceEnv)
	if dev == "" {
		t.Skipf("set %s=/dev/videoN to run tests against a real camera", testDeviceEnv)
	}
	wc, err := newCam(dev)
	if err != nil {
		t.Fatal(err)
	}
	saved, err := newPreset(wc)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		defer wc.Close()
		if err := applyPreset(wc, saved); err != nil {
			t.Errorf("restoring controls: %v", err)
		}
		checkPresetApplied(t, wc, saved, "after restoring controls")
	})
	return wc
}

// checkPresetApplied reports an error for every control in preset that is
// active and writable but does not have the preset's value.
func checkPresetApplied(t *testing.T, wc *Webcam, preset ConfigPreset, when string) {
	t.Helper()
	want := make(map[ControlID]int32)
	for _, v := range preset.Values {
		want[v.ID] = v.Value
	}
	for _, c := range wc.getControls() {
		v, ok := want[c.ID]
		if !ok || c.IsInactive() || c.IsReadOnly() {
			continue
		}
		if c.Value != v {
			t.Errorf("%s: %s = %d, want %d", when, c.Name, c.Value, v)
		}
	}
}

func findControl(wc *Webcam, id ControlID) (Control, bool) {
	controls := wc.getControls()
	i := slices.IndexFunc(controls, func(c Control) bool { return c.ID == id })
	if i < 0 {
		return Control{}, false
	}
	return controls[i], true
}

func mustControl(t *testing.T, wc *Webcam, id ControlID) Control {
	t.Helper()
	c, ok := findControl(wc, id)
	if !ok {
		t.Fatalf("control %#x not found", id)
	}
	return c
}

func mustSet(t *testing.T, wc *Webcam, id ControlID, value int32) {
	t.Helper()
	if err := wc.SetControl(id, value); err != nil {
		t.Fatalf("SetControl(%s, %d): %v", mustControl(t, wc, id).Name, value, err)
	}
}

// otherValue returns a valid value for c that differs from its current
// value, or false if c has only one valid value.
func otherValue(t *testing.T, wc *Webcam, c Control) (int32, bool) {
	t.Helper()
	var candidates []int32
	if c.Type == c_menu {
		candidates = menuValues(t, wc, c)
	} else {
		step := max(1, c.Step)
		top := c.Max - (c.Max-c.Min)%step // largest value on the step grid
		mid := c.Min + (c.Max-c.Min)/2/step*step
		candidates = []int32{mid, c.Min, top}
	}
	for _, v := range candidates {
		if v != c.Value {
			return v, true
		}
	}
	return 0, false
}

// presetWith returns the camera's current values with overrides applied.
func presetWith(t *testing.T, wc *Webcam, overrides map[ControlID]int32) ConfigPreset {
	t.Helper()
	p, err := newPreset(wc)
	if err != nil {
		t.Fatal(err)
	}
	for i, v := range p.Values {
		if o, ok := overrides[v.ID]; ok {
			p.Values[i].Value = o
		}
	}
	return p
}

// pairsOnDevice returns the pairs from autoPairs that the camera has, with on
// narrowed to the first value the camera supports.
func pairsOnDevice(t *testing.T, wc *Webcam) []autoPair {
	t.Helper()
	var pairs []autoPair
	for _, p := range autoPairs {
		auto, ok := findControl(wc, p.auto)
		if !ok {
			continue
		}
		if _, ok := findControl(wc, p.manual); !ok {
			continue
		}
		valid := []int32{auto.Min, auto.Max}
		if auto.Type == c_menu {
			valid = menuValues(t, wc, auto)
		}
		if !slices.Contains(valid, p.off) {
			continue
		}
		var on int32 = -1
		for _, v := range valid {
			if v != p.off && (len(p.on) == 0 || slices.Contains(p.on, v)) {
				on = v
				break
			}
		}
		if on < 0 {
			continue
		}
		p.on = []int32{on}
		pairs = append(pairs, p)
	}
	if len(pairs) == 0 {
		t.Skip("camera has none of the known automatic-mode controls")
	}
	return pairs
}

func TestDeviceOpenErrors(t *testing.T) {
	if os.Getenv(testDeviceEnv) == "" {
		t.Skipf("set %s=/dev/videoN to run tests against a real camera", testDeviceEnv)
	}
	if _, err := newCam("/dev/null"); err == nil {
		t.Error("newCam(/dev/null) succeeded, want error for non-V4L2 device")
	}
	if _, err := newCam("/dev/does-not-exist"); err == nil {
		t.Error("newCam(/dev/does-not-exist) succeeded, want error")
	}
}

func TestDeviceControls(t *testing.T) {
	wc := openTestCam(t)
	controls := wc.getControls()
	if len(controls) == 0 {
		t.Fatal("device has no controls")
	}
	seen := make(map[ControlID]bool)
	for i, c := range controls {
		t.Logf("%-32s id=%#08x type=%d flags=%#x value=%d range=[%d, %d] step=%d",
			c.Name, c.ID, c.Type, c.Flags, c.Value, c.Min, c.Max, c.Step)
		if seen[c.ID] {
			t.Errorf("%s: duplicate control ID %#x", c.Name, c.ID)
		}
		seen[c.ID] = true
		if i > 0 && controls[i-1].ID > c.ID {
			t.Errorf("controls not sorted by ID: %#x before %#x", controls[i-1].ID, c.ID)
		}
		if c.Name == "" {
			t.Errorf("control %#x has no name", c.ID)
		}
		if c.Flags&v4l2CtrlFlagDisabled != 0 {
			t.Errorf("%s: disabled control was listed", c.Name)
		}
		if c.Min > c.Max {
			t.Errorf("%s: min %d > max %d", c.Name, c.Min, c.Max)
		}
		if c.Value < c.Min || c.Value > c.Max {
			t.Errorf("%s: value %d outside [%d, %d]", c.Name, c.Value, c.Min, c.Max)
		}
		if c.Type == c_bool && (c.Min != 0 || c.Max != 1) {
			t.Errorf("%s: boolean with range [%d, %d]", c.Name, c.Min, c.Max)
		}
		if c.Type == c_menu && !slices.Contains(menuValues(t, wc, c), c.Value) {
			t.Errorf("%s: value %d is not a valid menu item", c.Name, c.Value)
		}
		v, err := wc.GetControl(c.ID)
		if err != nil {
			t.Errorf("%s: GetControl: %v", c.Name, err)
		} else if v != c.Value {
			t.Errorf("%s: GetControl = %d, getControls reported %d", c.Name, v, c.Value)
		}
	}
}

func TestDeviceUnknownControl(t *testing.T) {
	wc := openTestCam(t)
	const bogus ControlID = 0x00980fff
	if _, ok := findControl(wc, bogus); ok {
		t.Skipf("camera has control %#x", bogus)
	}
	if _, err := wc.GetControl(bogus); err == nil {
		t.Error("GetControl(unknown ID) succeeded")
	}
	if err := wc.SetControl(bogus, 1); err == nil {
		t.Error("SetControl(unknown ID) succeeded")
	}
}

// TestDeviceSetEachControl sets every active, writable control to another
// valid value, reads it back, and puts the original value back.
func TestDeviceSetEachControl(t *testing.T) {
	wc := openTestCam(t)
	for _, c := range wc.getControls() {
		t.Run(c.Name, func(t *testing.T) {
			// Earlier subtests may have changed flags and values.
			c := mustControl(t, wc, c.ID)
			if c.IsReadOnly() || c.IsInactive() {
				t.Skipf("flags %#x", c.Flags)
			}
			v, ok := otherValue(t, wc, c)
			if !ok {
				t.Skip("only one valid value")
			}
			mustSet(t, wc, c.ID, v)
			if got := mustControl(t, wc, c.ID).Value; got != v {
				t.Errorf("after setting %d, value = %d", v, got)
			}
			mustSet(t, wc, c.ID, c.Value)
			if got := mustControl(t, wc, c.ID).Value; got != c.Value {
				t.Errorf("after setting back %d, value = %d", c.Value, got)
			}
		})
	}
}

// TestDeviceInactiveFlag checks that the driver flags a control inactive
// exactly while its automatic mode is on. applyPreset depends on this.
func TestDeviceInactiveFlag(t *testing.T) {
	wc := openTestCam(t)
	for _, p := range pairsOnDevice(t, wc) {
		t.Run(p.name, func(t *testing.T) {
			mustSet(t, wc, p.auto, p.on[0])
			if c := mustControl(t, wc, p.manual); !c.IsInactive() {
				t.Errorf("%s active (flags %#x) with automatic mode on", c.Name, c.Flags)
			}
			mustSet(t, wc, p.auto, p.off)
			if c := mustControl(t, wc, p.manual); c.IsInactive() {
				t.Errorf("%s inactive (flags %#x) with automatic mode off", c.Name, c.Flags)
			}
		})
	}
}

func TestDeviceApplyPresetCurrentIsNoop(t *testing.T) {
	wc := openTestCam(t)
	p := presetWith(t, wc, nil)
	if err := applyPreset(wc, p); err != nil {
		t.Fatal(err)
	}
	checkPresetApplied(t, wc, p, "after applying current values")
}

// TestDeviceApplyPresetAutoToManual applies a preset that turns an automatic
// mode off and sets the control it was driving, starting with the automatic
// mode on. The control only becomes writable once the mode is off, so this
// fails if applyPreset sets controls in the wrong order.
func TestDeviceApplyPresetAutoToManual(t *testing.T) {
	wc := openTestCam(t)
	for _, p := range pairsOnDevice(t, wc) {
		t.Run(p.name, func(t *testing.T) {
			// Pick the target value while the control is writable.
			mustSet(t, wc, p.auto, p.off)
			target, ok := otherValue(t, wc, mustControl(t, wc, p.manual))
			if !ok {
				t.Skip("manual control has only one valid value")
			}
			mustSet(t, wc, p.auto, p.on[0])

			preset := presetWith(t, wc, map[ControlID]int32{p.auto: p.off, p.manual: target})
			if err := applyPreset(wc, preset); err != nil {
				t.Fatal(err)
			}
			if got := mustControl(t, wc, p.auto).Value; got != p.off {
				t.Errorf("automatic mode = %d, want %d", got, p.off)
			}
			m := mustControl(t, wc, p.manual)
			if m.IsInactive() {
				t.Errorf("%s still inactive", m.Name)
			}
			if m.Value != target {
				t.Errorf("%s = %d, want %d", m.Name, m.Value, target)
			}
		})
	}
}

// TestDeviceApplyPresetManualToAuto applies a preset that turns an automatic
// mode on and also holds a value for the control it drives. That value can't
// be applied, and must not be reported as an error.
func TestDeviceApplyPresetManualToAuto(t *testing.T) {
	wc := openTestCam(t)
	for _, p := range pairsOnDevice(t, wc) {
		t.Run(p.name, func(t *testing.T) {
			mustSet(t, wc, p.auto, p.off)
			target, ok := otherValue(t, wc, mustControl(t, wc, p.manual))
			if !ok {
				t.Skip("manual control has only one valid value")
			}

			preset := presetWith(t, wc, map[ControlID]int32{p.auto: p.on[0], p.manual: target})
			if err := applyPreset(wc, preset); err != nil {
				t.Fatal(err)
			}
			if got := mustControl(t, wc, p.auto).Value; got != p.on[0] {
				t.Errorf("automatic mode = %d, want %d", got, p.on[0])
			}
			if m := mustControl(t, wc, p.manual); !m.IsInactive() {
				t.Errorf("%s active (flags %#x) with automatic mode on", m.Name, m.Flags)
			}
		})
	}
}

// TestDeviceApplyPresetAllManual turns every automatic mode on, then applies
// a preset with all of them off and every manual control changed at once.
func TestDeviceApplyPresetAllManual(t *testing.T) {
	wc := openTestCam(t)
	pairs := pairsOnDevice(t, wc)
	overrides := make(map[ControlID]int32)
	for _, p := range pairs {
		mustSet(t, wc, p.auto, p.off)
		if v, ok := otherValue(t, wc, mustControl(t, wc, p.manual)); ok {
			overrides[p.manual] = v
		}
		overrides[p.auto] = p.off
	}
	for _, p := range pairs {
		mustSet(t, wc, p.auto, p.on[0])
	}

	preset := presetWith(t, wc, overrides)
	if err := applyPreset(wc, preset); err != nil {
		t.Fatal(err)
	}
	checkPresetApplied(t, wc, preset, "after applying preset")
	for _, p := range pairs {
		if m := mustControl(t, wc, p.manual); m.IsInactive() {
			t.Errorf("%s still inactive", m.Name)
		}
	}
}

// TestDeviceApplyPresetRoundTrip saves a preset, changes every control it
// can, and checks that applying the preset brings the values back.
func TestDeviceApplyPresetRoundTrip(t *testing.T) {
	wc := openTestCam(t)
	// Start with automatic modes off so the saved preset has manual values
	// that must be restored, and save it.
	pairs := pairsOnDevice(t, wc)
	for _, p := range pairs {
		mustSet(t, wc, p.auto, p.off)
	}
	saved := presetWith(t, wc, nil)

	// Change every writable control, then turn the automatic modes on so
	// the manual controls are locked when the preset is applied.
	for _, c := range wc.getControls() {
		c := mustControl(t, wc, c.ID)
		if c.IsReadOnly() || c.IsInactive() {
			continue
		}
		if v, ok := otherValue(t, wc, c); ok {
			if err := wc.SetControl(c.ID, v); err != nil {
				t.Logf("changing %s to %d: %v", c.Name, v, err)
			}
		}
	}
	for _, p := range pairs {
		mustSet(t, wc, p.auto, p.on[0])
	}

	if err := applyPreset(wc, saved); err != nil {
		t.Fatal(err)
	}
	checkPresetApplied(t, wc, saved, "after applying saved preset")
	for _, p := range pairs {
		if m := mustControl(t, wc, p.manual); m.IsInactive() {
			t.Errorf("%s inactive, but the saved preset had automatic mode off", m.Name)
		}
	}
}

// TestDeviceApplyPresetPartial checks that controls missing from a preset,
// and controls the camera doesn't have, are left alone.
func TestDeviceApplyPresetPartial(t *testing.T) {
	wc := openTestCam(t)
	before := wc.getControls()
	var target Control
	var value int32
	for _, c := range before {
		if c.Type == c_int && !c.IsReadOnly() && !c.IsInactive() {
			if v, ok := otherValue(t, wc, c); ok {
				target, value = c, v
				break
			}
		}
	}
	if target.Name == "" {
		t.Skip("no writable integer control")
	}

	preset := ConfigPreset{Values: []ConfigPresetValue{
		{ID: target.ID, Value: value},
		{ID: 0x00980fff, Value: 1}, // not a control on the camera
	}}
	if err := applyPreset(wc, preset); err != nil {
		t.Fatal(err)
	}
	for _, c := range before {
		got := mustControl(t, wc, c.ID)
		switch {
		case c.ID == target.ID:
			if got.Value != value {
				t.Errorf("%s = %d, want %d", c.Name, got.Value, value)
			}
		case got.Value != c.Value:
			t.Errorf("%s changed from %d to %d, but is not in the preset", c.Name, c.Value, got.Value)
		}
	}
}
