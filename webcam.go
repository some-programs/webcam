package main

import (
	"fmt"
	"math"
	"sort"
	"strings"

	"github.com/blackjack/webcam"
	"github.com/evertras/bubble-table/table"
)

type ControlID = webcam.ControlID

// Control .
type Control struct {
	ID    ControlID
	Name  string
	Min   int32
	Max   int32
	Value int32
	Step  int32
	Type  controlType
	Flags uint32
}

// TODO temporary copy

type controlType int

const (
	c_int controlType = iota
	c_bool
	c_menu
)

func (c Control) String() string {
	return fmt.Sprintf("(id:%v val:%v min:%v max:%v name:%v)", c.ID, c.Value, c.Min, c.Max, c.Name)
}

func (c Control) IsBoolean() bool {
	return c.Type == c_bool || (c.Min == 0 && c.Max == 1)
}

func (c Control) IsMenu() bool {
	return c.Type == c_menu

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
		return min(c.Max, c.Value+1)
	default:
		return c.GetStepsChange(1)
	}
}

func (c Control) GetValueDecreseStep() int32 {
	switch {
	case c.IsBoolean():
		return c.ToggleBoolean()
	case c.IsMenu():
		return max(c.Min, c.Value-1)
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
		return min(c.Max, c.Value+1)
	default:
		return c.GetPercentChange(0.02)
	}
}

func (c Control) GetValueDecreasePercent() int32 {
	switch {
	case c.IsBoolean():
		return c.ToggleBoolean()
	case c.IsMenu():
		return max(c.Min, c.Value-1)
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
	return (float64(c.Value) - float64(c.Min)) / (float64(c.Max) - float64(c.Min))
}

func (c Control) Row(barWidth int) table.Row {
	var bar string

	switch {
	case c.IsBoolean():
		if c.Value == 0 {
			bar = "OFF"
		} else {
			bar = "ON"
		}
	case c.IsMenu():
		bar = fmt.Sprintf("%v", c.Value)
	default:
		bar = percentBar(barWidth, c.Percent())
	}
	return table.NewRow(table.RowData{
		"name":  c.Name,
		"min":   c.Min,
		"max":   c.Max,
		"value": c.Value,
		"bar":   bar,
		// hidden
		"control": c,
	})

}

func newCam(device string) (*Webcam, error) {
	cam, err := webcam.Open(device)
	if err != nil {
		return nil, err
	}
	return &Webcam{
		webcam: cam,
	}, nil

}

type Webcam struct {
	webcam *webcam.Webcam
}

func (wc *Webcam) getControls() []Control {
	controlsMap := wc.webcam.GetControls()
	var controls []Control
	for ci, c := range controlsMap {
		value, err := wc.webcam.GetControl(ci)
		if err != nil {
			panic(err)
		}
		controls = append(controls,
			Control{
				ID:    ci,
				Name:  c.Name,
				Min:   c.Min,
				Max:   c.Max,
				Step:  c.Step,
				Type:  controlType(c.Type),
				Value: value,
			})
	}
	sort.Slice(controls, func(i, j int) bool {
		return controls[i].ID < controls[j].ID
	})
	return controls
}

func (wc *Webcam) Close() error {
	return wc.webcam.Close()

}

func percentBar(width int, percent float64) string {
	w := float64(width)
	fullSize := int(math.Round(w * percent))
	emptySize := int(w) - fullSize
	return strings.Repeat("█", fullSize) + strings.Repeat("░", emptySize)
}

func controlValue(control Control, percent float64) int32 {
	return int32(
		min(float64(control.Max),
			max((float64(control.Min)),
				((float64(control.Max)-float64(control.Min))*percent)+float64(control.Min))))
}
