package main

import (
	"cmp"
	"fmt"
	"math"
	"slices"

	"golang.org/x/sys/unix"
)

// Control .
type Control struct {
	ID      ControlID
	Name    string
	Min     int32
	Max     int32
	Value   int32
	Default int32
	Step    int32
	Type    controlType
	Flags   uint32
	// Menu holds the items of a menu control in value order. Its values may
	// have gaps.
	Menu []MenuItem
}

// MenuItem is one choice of a menu control.
type MenuItem struct {
	Value int32
	Name  string
}

type controlType int

const (
	c_int controlType = iota
	c_bool
	c_menu
)

func (c Control) String() string {
	return fmt.Sprintf("(id:%v val:%v min:%v max:%v name:%v)", c.ID, c.Value, c.Min, c.Max, c.Name)
}

// IsBoolean reports whether the control is a boolean or an integer that can
// only be 0 or 1.
func (c Control) IsBoolean() bool {
	return c.Type == c_bool || (c.Type == c_int && c.Min == 0 && c.Max == 1)
}

// IsInactive reports whether the control currently has no effect, typically
// because an automatic mode such as auto exposure controls its value.
func (c Control) IsInactive() bool {
	return c.Flags&v4l2CtrlFlagInactive != 0
}

// IsReadOnly reports whether the control can not be set.
func (c Control) IsReadOnly() bool {
	return c.Flags&v4l2CtrlFlagReadOnly != 0
}

func (c Control) IsMenu() bool {
	return c.Type == c_menu

}

// MenuName returns the name of the menu item for value, or the value as a
// number if the menu has no such item.
func (c Control) MenuName(value int32) string {
	i, ok := c.menuIndex(value)
	if !ok {
		return fmt.Sprint(value)
	}
	return c.Menu[i].Name
}

// menuIndex returns the index of the menu item for value.
func (c Control) menuIndex(value int32) (int, bool) {
	return slices.BinarySearchFunc(c.Menu, value, func(item MenuItem, v int32) int {
		return cmp.Compare(item.Value, v)
	})
}

// menuStep returns the value of the menu item steps items away from the
// current value, stopping at the first and last item.
func (c Control) menuStep(steps int) int32 {
	if len(c.Menu) == 0 {
		return min(c.Max, max(c.Min, c.Value+int32(steps)))
	}
	// If the current value has no item, i is where it would be.
	i, ok := c.menuIndex(c.Value)
	if !ok && steps > 0 {
		i--
	}
	i = min(len(c.Menu)-1, max(0, i+steps))
	return c.Menu[i].Value
}

// MenuItemValue returns the value of the menu item at index i.
func (c Control) MenuItemValue(i int) (int32, bool) {
	if i < 0 || i >= len(c.Menu) {
		return 0, false
	}
	return c.Menu[i].Value, true
}

func (c Control) ToggleBoolean() int32 {
	if c.Value == c.Min {
		return c.Max
	}
	return c.Min
}

func (c Control) GetValueIncreaseStep() int32 {
	switch {
	case c.IsBoolean():
		return c.ToggleBoolean()
	case c.IsMenu():
		return c.menuStep(1)
	default:
		return c.GetStepsChange(1)
	}
}

func (c Control) GetValueDecreseStep() int32 {
	switch {
	case c.IsBoolean():
		return c.ToggleBoolean()
	case c.IsMenu():
		return c.menuStep(-1)
	default:
		return c.GetStepsChange(-1)
	}
}

func (c Control) GetStepsChange(steps int32) int32 {
	stepSize := max(1, c.Step)
	nextValue := c.Value + steps*stepSize
	switch {
	case steps < 0:
		nextValue = max(nextValue, c.Min)
	case steps > 0:
		nextValue = min(nextValue, c.Max)
	}
	return nextValue
}

func (c Control) GetValueIncreasePercent() int32 {
	switch {
	case c.IsBoolean():
		return c.ToggleBoolean()
	case c.IsMenu():
		return c.menuStep(1)
	default:
		return c.GetPercentChange(0.02)
	}
}

func (c Control) GetValueDecreasePercent() int32 {
	switch {
	case c.IsBoolean():
		return c.ToggleBoolean()
	case c.IsMenu():
		return c.menuStep(-1)
	default:
		return c.GetPercentChange(-0.02)
	}
}

func (c Control) GetPercentChange(add float64) int32 {
	stepSize := max(1, c.Step)
	prevPercent := c.Percent()
	nextPercent := prevPercent + add
	span := float64(c.Max) - float64(c.Min)
	nextValue := int32(math.Round(span*nextPercent + float64(c.Min)))
	switch {
	case add < 0:
		nextValue = min(nextValue, c.Value-stepSize)
		nextValue = max(nextValue, c.Min)
	case add > 0:
		nextValue = max(nextValue, c.Value+stepSize)
		nextValue = min(nextValue, c.Max)
	}
	return nextValue
}

func (c Control) Percent() float64 {
	return c.percentOf(c.Value)
}

func (c Control) percentOf(value int32) float64 {
	return (float64(value) - float64(c.Min)) / (float64(c.Max) - float64(c.Min))
}

// formatValue returns value as shown in the bar column.
func (c Control) formatValue(value int32) string {
	switch {
	case c.IsBoolean():
		if value == 0 {
			return "off"
		}
		return "on"
	case c.IsMenu():
		return c.MenuName(value)
	default:
		return fmt.Sprint(value)
	}
}

// state returns a short note on why the control can not be changed, if it
// can not.
func (c Control) state() string {
	switch {
	case c.IsReadOnly():
		return "readonly"
	case c.IsInactive():
		return "inactive"
	default:
		return ""
	}
}

func newCam(device string) (*Webcam, error) {
	fd, c, err := openDevice(device)
	if err != nil {
		return nil, err
	}
	return &Webcam{
		fd:     fd,
		Path:   device,
		Card:   cString(c.Card[:]),
		Driver: cString(c.Driver[:]),
		menus:  make(map[ControlID][]MenuItem),

		classNames: make(map[uint32]string),
	}, nil
}

// Webcam is an open V4L2 device.
type Webcam struct {
	fd     int
	Path   string // device file, such as /dev/video0
	Card   string // device name reported by the driver
	Driver string

	// menus caches the items of each menu control.
	menus map[ControlID][]MenuItem
	// classNames holds the driver's name for each control class, such as
	// "Camera Controls".
	classNames map[uint32]string
}

// controlClass returns the class of control id, such as
// V4L2_CTRL_CLASS_CAMERA.
func controlClass(id ControlID) uint32 {
	return id & 0x0fff0000
}

// ClassName returns the name of a control class.
func (wc *Webcam) ClassName(class uint32) string {
	if name, ok := wc.classNames[class]; ok {
		return name
	}
	return fmt.Sprintf("Class %#x", class)
}

func (wc *Webcam) getControls() []Control {
	queried, err := queryControls(wc.fd)
	if err != nil {
		panic(err)
	}
	var controls []Control
	for _, q := range queried {
		if q.Type == v4l2CtrlTypeClass {
			wc.classNames[controlClass(q.ID)] = cString(q.Name[:])
			continue
		}
		value, err := wc.GetControl(q.ID)
		if err != nil {
			panic(err)
		}
		c := Control{
			ID:      q.ID,
			Name:    cString(q.Name[:]),
			Min:     int32(q.Minimum),
			Max:     int32(q.Maximum),
			Default: int32(q.DefaultValue),
			Step:    int32(q.Step),
			Type:    c_int,
			Flags:   q.Flags,
			Value:   value,
		}
		switch q.Type {
		case v4l2CtrlTypeBoolean:
			c.Type = c_bool
		case v4l2CtrlTypeMenu:
			c.Type = c_menu
			c.Menu = wc.menu(c)
		}
		controls = append(controls, c)
	}
	slices.SortFunc(controls, func(a, b Control) int {
		return cmp.Compare(a.ID, b.ID)
	})
	return controls
}

// menu returns the items of menu control c.
func (wc *Webcam) menu(c Control) []MenuItem {
	if items, ok := wc.menus[c.ID]; ok {
		return items
	}
	items, err := queryMenu(wc.fd, c.ID, c.Min, c.Max)
	if err != nil {
		panic(err)
	}
	wc.menus[c.ID] = items
	return items
}

// GetControl returns the current value of control id.
func (wc *Webcam) GetControl(id ControlID) (int32, error) {
	return getControl(wc.fd, id)
}

// SetControl sets control id to value.
func (wc *Webcam) SetControl(id ControlID, value int32) error {
	return setControl(wc.fd, id, value)
}

func (wc *Webcam) Close() error {
	return unix.Close(wc.fd)
}

func controlValue(control Control, percent float64) int32 {
	return int32(
		min(float64(control.Max),
			max((float64(control.Min)),
				((float64(control.Max)-float64(control.Min))*percent)+float64(control.Min))))
}
