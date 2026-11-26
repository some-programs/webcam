# webcam

TUI webcam configuration utility for LINUX.

**Work in progress**


```
 C922 Pro Stream Webcam · /dev/video0 · uvcvideo

 ── User Controls ────────────────────────────────────────────────────────────────────────────────────────────────
   Brightness                   ████████████████████████████████│                                     128
   Contrast                     ████████████████████████████████│                                     128
   Saturation                   ████████████████████████████████│                                     128
   White Balance, Automatic     ○ off
   Gain                         │                                                                       0
   Power Line Frequency         ‹ 50 Hz ›
   White Balance Temperature    ███████████████████▎        │                                        3355
   Sharpness                    ████████████████████████████████│                                     128
   Backlight Compensation       ● on
 ── Camera Controls ──────────────────────────────────────────────────────────────────────────────────────────────
   Auto Exposure                  Manual Mode ›
 ▸ Exposure Time, Absolute      ███████│████▌                                                         404
   Exposure, Dynamic Framerate  ● on
   Pan, Absolute                ████████████████████████████████│                                       0
   Tilt, Absolute               ████████████████████████████████│                                       0
   Focus, Absolute              │                                                                       0 inactive
   Focus, Automatic Continuous  ● on
   Zoom, Absolute               │▎                                                                    108

 Exposure Time, Absolute: 404 · default 250 · range 3‥2047
                                                                                               presets F1 F4 F5 F6
 ↑/↓ select    →/l increase                  r          reset to default
 /   filter    ←/h decrease                  R          reset all
 ?   less      o   step up                   F1-F12     load preset
 q   quit      y   step down                 alt+F1-F12 save preset
               `…0 set 0-100% / menu item
```

The mouse works too: click a row to select it, click or drag on a bar to set
its value, click a switch or menu to change it, and use the wheel over a row to
adjust it.
