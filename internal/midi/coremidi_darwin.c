// CoreMIDI input: one client and one input port connected to every source.
// MIDI 1.0 arrives as Universal MIDI Packets; channel voice (type 2) and
// system (type 1) words are handed to Go as status/data bytes.

#include <CoreMIDI/CoreMIDI.h>
#include "coremidi_darwin.h"
#include "_cgo_export.h"

static MIDIClientRef gClient;
static MIDIPortRef gPort;

// umpWords is the size of a UMP message from its type nibble.
static UInt32 umpWords(UInt32 type) {
	switch (type) {
	case 0x0: case 0x1: case 0x2: case 0x6: case 0x7: return 1;
	case 0x3: case 0x4: case 0x8: case 0x9: case 0xA: return 2;
	case 0xB: case 0xC: return 3;
	default: return 4;
	}
}

int pl_midi_open(void) {
	if (gClient) return 0;
	OSStatus st = MIDIClientCreateWithBlock(CFSTR("Punchline"), &gClient, ^(const MIDINotification *n) {
		if (n->messageID == kMIDIMsgSetupChanged) goMIDISetupChanged();
	});
	if (st != noErr) return st;
	st = MIDIInputPortCreateWithProtocol(gClient, CFSTR("Punchline In"), kMIDIProtocol_1_0, &gPort,
		^(const MIDIEventList *list, void *ref) {
			uint32_t src = (uint32_t)(uintptr_t)ref;
			const MIDIEventPacket *p = &list->packet[0];
			for (UInt32 i = 0; i < list->numPackets; i++) {
				for (UInt32 w = 0; w < p->wordCount;) {
					UInt32 word = p->words[w];
					UInt32 type = word >> 28;
					if (type == 0x1 || type == 0x2)
						goMIDIEvent(src, (uint8_t)(word >> 16), (uint8_t)(word >> 8), (uint8_t)word);
					w += umpWords(type);
				}
				p = MIDIEventPacketNext(p);
			}
		});
	if (st != noErr) {
		MIDIClientDispose(gClient);
		gClient = 0;
	}
	return st;
}

void pl_midi_close(void) {
	if (gPort) MIDIPortDispose(gPort);
	if (gClient) MIDIClientDispose(gClient);
	gPort = 0;
	gClient = 0;
}

int pl_midi_source_count(void) {
	return (int)MIDIGetNumberOfSources();
}

int pl_midi_source(int i, int32_t *uid, char *name, int n) {
	MIDIEndpointRef e = MIDIGetSource((ItemCount)i);
	if (!e) return -1;
	SInt32 u = 0;
	MIDIObjectGetIntegerProperty(e, kMIDIPropertyUniqueID, &u);
	*uid = u;
	name[0] = 0;
	CFStringRef s = NULL;
	if (MIDIObjectGetStringProperty(e, kMIDIPropertyDisplayName, &s) == noErr && s) {
		CFStringGetCString(s, name, n, kCFStringEncodingUTF8);
		CFRelease(s);
	}
	return 0;
}

int pl_midi_connect(int i, int32_t uid) {
	MIDIEndpointRef e = MIDIGetSource((ItemCount)i);
	if (!e) return kMIDIObjectNotFound;
	return MIDIPortConnectSource(gPort, e, (void *)(uintptr_t)(uint32_t)uid);
}

void pl_midi_run(void) {
	CFRunLoopRun();
}
