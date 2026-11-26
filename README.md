# webcam

TUI webcam configuration utility for LINUX.

**Work in progress**


```
 C922 Pro Stream Webcam · /dev/video0 · uvcvideo
 ┏━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━┳━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━┳━━━━━━━┳━━━━━━━┳━━━━━━┳━━━━━━┳━━━━━━━━┓
 ┃                          Name┃                                        ┃Value  ┃Default┃   Min┃   Max┃        ┃
 ┣━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━╋━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━╋━━━━━━━╋━━━━━━━╋━━━━━━╋━━━━━━╋━━━━━━━━┫
 ┃                    Brightness┃████████████████████│░░░░░░░░░░░░░░░░░░░┃128    ┃    128┃     0┃   255┃        ┃
 ┃                      Contrast┃████████████████████│░░░░░░░░░░░░░░░░░░░┃128    ┃    128┃     0┃   255┃        ┃
 ┃                    Saturation┃████████████████████│░░░░░░░░░░░░░░░░░░░┃128    ┃    128┃     0┃   255┃        ┃
 ┃      White Balance, Automatic┃                   ON                   ┃1      ┃      1┃     0┃     1┃        ┃
 ┃                          Gain┃│░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░┃0      ┃      0┃     0┃   255┃        ┃
 ┃          Power Line Frequency┃                 60 Hz                  ┃2      ┃      2┃     0┃     2┃        ┃
 ┃     White Balance Temperature┃█████████████████│████████░░░░░░░░░░░░░░┃4900   ┃   4000┃  2000┃  6500┃inactive┃
 ┃                     Sharpness┃████████████████████│░░░░░░░░░░░░░░░░░░░┃128    ┃    128┃     0┃   255┃        ┃
 ┃        Backlight Compensation┃                  OFF                   ┃0      ┃      0┃     0┃     1┃        ┃
 ┃                 Auto Exposure┃         Aperture Priority Mode         ┃3      ┃      3┃     0┃     3┃        ┃
 ┃       Exposure Time, Absolute┃████│█████████░░░░░░░░░░░░░░░░░░░░░░░░░░┃737    ┃    250┃     3┃  2047┃inactive┃
 ┃   Exposure, Dynamic Framerate┃                  OFF                   ┃0      ┃      0┃     0┃     1┃        ┃
 ┃                 Pan, Absolute┃████████████████████│░░░░░░░░░░░░░░░░░░░┃0      ┃      0┃-36000┃ 36000┃        ┃
 ┃                Tilt, Absolute┃████████████████████│░░░░░░░░░░░░░░░░░░░┃0      ┃      0┃-36000┃ 36000┃        ┃
 ┃               Focus, Absolute┃│░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░┃0      ┃      0┃     0┃   250┃inactive┃
 ┃   Focus, Automatic Continuous┃                   ON                   ┃1      ┃      1┃     0┃     1┃        ┃
 ┃                Zoom, Absolute┃│░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░┃100    ┃    100┃   100┃   500┃        ┃
 ┣━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━┻━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━┻━━━━━━━┻━━━━━━━┻━━━━━━┻━━━━━━┻━━━━━━━━┫
 ┃                                                                                                              ┃
 ┗━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━┛
                                                                                              presets F1 F4 F5 F6
 ↑/↓ select    →/l increase                  r          reset to default
 /   filter    ←/h decrease                  R          reset all
 ?   less      o   step up                   F1-F12     load preset
 q   quit      y   step down                 alt+F1-F12 save preset
               `…0 set 0-100% / menu item
```
