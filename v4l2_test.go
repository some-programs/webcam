package main

import (
	"testing"
	"unsafe"
)

func TestV4L2ABI(t *testing.T) {
	sizes := []struct {
		name string
		got  uintptr
		want uintptr
	}{
		{"v4l2_capability", unsafe.Sizeof(v4l2Capability{}), 104},
		{"v4l2_control", unsafe.Sizeof(v4l2Control{}), 8},
		{"v4l2_query_ext_ctrl", unsafe.Sizeof(v4l2QueryExtCtrl{}), 232},
	}
	for _, s := range sizes {
		if s.got != s.want {
			t.Errorf("sizeof(struct %s) = %d, want %d", s.name, s.got, s.want)
		}
	}

	// Values from linux/videodev2.h as compiled on Linux.
	reqs := []struct {
		name string
		got  uintptr
		want uintptr
	}{
		{"VIDIOC_QUERYCAP", vidiocQueryCap, 0x80685600},
		{"VIDIOC_G_CTRL", vidiocGCtrl, 0xc008561b},
		{"VIDIOC_S_CTRL", vidiocSCtrl, 0xc008561c},
		{"VIDIOC_QUERY_EXT_CTRL", vidiocQueryExtCtrl, 0xc0e85667},
	}
	for _, r := range reqs {
		if r.got != r.want {
			t.Errorf("%s = %#x, want %#x", r.name, r.got, r.want)
		}
	}
}
