package main

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"unsafe"

	"golang.org/x/sys/unix"
)

// This file talks to V4L2 devices directly with ioctl(2). The structs mirror
// the C layouts in linux/videodev2.h; v4l2_test.go checks their sizes and the
// resulting request numbers against the kernel's values.

// ControlID identifies a V4L2 control, such as V4L2_CID_BRIGHTNESS.
type ControlID = uint32

// V4L2 control types (enum v4l2_ctrl_type).
const (
	v4l2CtrlTypeInteger = 1
	v4l2CtrlTypeBoolean = 2
	v4l2CtrlTypeMenu    = 3
	v4l2CtrlTypeClass   = 6
)

// V4L2 control flags (V4L2_CTRL_FLAG_*).
const (
	v4l2CtrlFlagDisabled = 0x0001
	v4l2CtrlFlagReadOnly = 0x0004
	v4l2CtrlFlagInactive = 0x0010
	v4l2CtrlFlagNextCtrl = 0x80000000
)

// struct v4l2_capability
type v4l2Capability struct {
	Driver       [16]byte
	Card         [32]byte
	BusInfo      [32]byte
	Version      uint32
	Capabilities uint32
	DeviceCaps   uint32
	Reserved     [3]uint32
}

// struct v4l2_control
type v4l2Control struct {
	ID    uint32
	Value int32
}

// struct v4l2_querymenu
type v4l2QueryMenu struct {
	ID       uint32
	Index    uint32
	Name     [32]byte // union with int64 value; the C struct is packed
	Reserved uint32
}

// struct v4l2_query_ext_ctrl
type v4l2QueryExtCtrl struct {
	ID           uint32
	Type         uint32
	Name         [32]byte
	Minimum      int64
	Maximum      int64
	Step         uint64
	DefaultValue int64
	Flags        uint32
	ElemSize     uint32
	Elems        uint32
	NrOfDims     uint32
	Dims         [4]uint32
	Reserved     [32]uint32
}

// ioc encodes an ioctl request number like the kernel's _IOC macro.
func ioc(dir, nr, size uintptr) uintptr {
	const (
		nrShift   = 0
		typeShift = 8
		sizeShift = 16
		dirShift  = 30
	)
	return dir<<dirShift | size<<sizeShift | 'V'<<typeShift | nr<<nrShift
}

const (
	iocRead      = 2
	iocReadWrite = 3
)

var (
	vidiocQueryCap     = ioc(iocRead, 0, unsafe.Sizeof(v4l2Capability{}))
	vidiocGCtrl        = ioc(iocReadWrite, 27, unsafe.Sizeof(v4l2Control{}))
	vidiocSCtrl        = ioc(iocReadWrite, 28, unsafe.Sizeof(v4l2Control{}))
	vidiocQueryMenu    = ioc(iocReadWrite, 37, unsafe.Sizeof(v4l2QueryMenu{}))
	vidiocQueryExtCtrl = ioc(iocReadWrite, 103, unsafe.Sizeof(v4l2QueryExtCtrl{}))
)

func ioctl(fd int, req uintptr, arg unsafe.Pointer) error {
	for {
		_, _, errno := unix.Syscall(unix.SYS_IOCTL, uintptr(fd), req, uintptr(arg))
		switch errno {
		case 0:
			return nil
		case unix.EINTR:
			continue
		default:
			return errno
		}
	}
}

// openDevice opens the V4L2 device at path and returns its capabilities,
// which also checks that it is a V4L2 device.
func openDevice(path string) (int, v4l2Capability, error) {
	var c v4l2Capability
	fd, err := unix.Open(path, unix.O_RDWR|unix.O_NONBLOCK|unix.O_CLOEXEC, 0)
	if err != nil {
		return -1, c, &os.PathError{Op: "open", Path: path, Err: err}
	}
	if err := ioctl(fd, vidiocQueryCap, unsafe.Pointer(&c)); err != nil {
		unix.Close(fd)
		if errors.Is(err, unix.ENOTTY) {
			return -1, c, fmt.Errorf("%s: not a V4L2 device", path)
		}
		return -1, c, &os.PathError{Op: "VIDIOC_QUERYCAP", Path: path, Err: err}
	}
	return fd, c, nil
}

// queryControls returns the device's enabled integer, boolean and menu
// controls in driver order, along with the class entries that name each
// group of controls.
func queryControls(fd int) ([]v4l2QueryExtCtrl, error) {
	var controls []v4l2QueryExtCtrl
	id := uint32(0)
	for {
		q := v4l2QueryExtCtrl{ID: id | v4l2CtrlFlagNextCtrl}
		err := ioctl(fd, vidiocQueryExtCtrl, unsafe.Pointer(&q))
		if errors.Is(err, unix.EINVAL) {
			// No more controls.
			return controls, nil
		}
		if err != nil {
			return nil, err
		}
		id = q.ID
		if q.Flags&v4l2CtrlFlagDisabled != 0 {
			continue
		}
		switch q.Type {
		case v4l2CtrlTypeInteger, v4l2CtrlTypeBoolean, v4l2CtrlTypeMenu, v4l2CtrlTypeClass:
			controls = append(controls, q)
		}
	}
}

// queryMenu returns the items of the menu control id, whose values range
// from minimum to maximum. Drivers may leave gaps in that range; values
// without an item are left out.
func queryMenu(fd int, id ControlID, minimum, maximum int32) ([]MenuItem, error) {
	var items []MenuItem
	for i := minimum; i <= maximum; i++ {
		q := v4l2QueryMenu{ID: id, Index: uint32(i)}
		err := ioctl(fd, vidiocQueryMenu, unsafe.Pointer(&q))
		if errors.Is(err, unix.EINVAL) {
			continue
		}
		if err != nil {
			return nil, err
		}
		items = append(items, MenuItem{Value: i, Name: cString(q.Name[:])})
	}
	return items, nil
}

func getControl(fd int, id ControlID) (int32, error) {
	c := v4l2Control{ID: id}
	if err := ioctl(fd, vidiocGCtrl, unsafe.Pointer(&c)); err != nil {
		return 0, err
	}
	return c.Value, nil
}

func setControl(fd int, id ControlID, value int32) error {
	c := v4l2Control{ID: id, Value: value}
	return ioctl(fd, vidiocSCtrl, unsafe.Pointer(&c))
}

// cString returns b up to its first NUL byte.
func cString(b []byte) string {
	s, _, _ := bytes.Cut(b, []byte{0})
	return string(s)
}
