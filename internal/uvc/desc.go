package uvc

// USB Video Class descriptor parsing and request constants, shared by
// backends that talk to the device directly (macOS). Pure Go and
// platform-independent so it can be unit tested anywhere.

const (
	usbDescInterface   = 0x04
	usbClassVideo      = 0x0e
	usbSubclassVC      = 0x01
	csInterface        = 0x24
	vcInputTerminal    = 0x02
	ittCamera          = 0x0201
	ctZoomAbsolute     = 0x0b // CT_ZOOM_ABSOLUTE_CONTROL selector
	ctZoomAbsoluteBit  = 9    // bit in the camera terminal's bmControls
	uvcSetCur          = 0x01
	uvcGetCur          = 0x81
	uvcGetMin          = 0x82
	uvcGetMax          = 0x83
	uvcGetRes          = 0x84
	uvcGetDef          = 0x87
	reqTypeClassIfOut  = 0x21 // host-to-device, class, interface
	reqTypeClassIfIn   = 0xa1 // device-to-host, class, interface
	zoomAbsoluteLength = 2    // wObjectiveFocalLength, unsigned 16-bit
)

// vcFunction is one Video Control interface found in a configuration
// descriptor.
type vcFunction struct {
	Interface  uint8 // bInterfaceNumber of the Video Control interface
	HasCamera  bool  // a camera input terminal was found
	TerminalID uint8 // its bTerminalID
	Zoom       bool  // the camera terminal advertises absolute zoom
}

// parseVideoControl lists the Video Control interfaces in a full
// configuration descriptor (as returned by GET_DESCRIPTOR(CONFIGURATION)),
// with each one's first camera terminal. Malformed trailing data is ignored.
func parseVideoControl(cfg []byte) []vcFunction {
	var out []vcFunction
	inVC := false
	for i := 0; i+2 <= len(cfg); {
		l := int(cfg[i])
		if l < 2 || i+l > len(cfg) {
			break
		}
		d := cfg[i : i+l]
		i += l
		switch d[1] {
		case usbDescInterface:
			if l < 9 {
				inVC = false
				continue
			}
			// Only alternate setting 0 carries the class-specific VC header.
			inVC = d[5] == usbClassVideo && d[6] == usbSubclassVC && d[3] == 0
			if inVC {
				out = append(out, vcFunction{Interface: d[2]})
			}
		case csInterface:
			if !inVC || l < 8 || d[2] != vcInputTerminal {
				continue
			}
			if uint16(d[4])|uint16(d[5])<<8 != ittCamera {
				continue
			}
			f := &out[len(out)-1]
			if f.HasCamera {
				continue
			}
			f.HasCamera, f.TerminalID = true, d[3]
			if l >= 15 {
				end := 15 + int(d[14])
				if end > l {
					end = l
				}
				ctrls := d[15:end]
				byteIdx, bit := ctZoomAbsoluteBit/8, ctZoomAbsoluteBit%8
				f.Zoom = len(ctrls) > byteIdx && ctrls[byteIdx]&(1<<bit) != 0
			}
		}
	}
	return out
}
