// Native half of the CoreMIDI backend (coremidi_darwin.c).
#include <stdint.h>

int pl_midi_open(void);  // OSStatus
void pl_midi_close(void);
int pl_midi_source_count(void);
// pl_midi_source fills uid and name for source i; returns 0 on success.
int pl_midi_source(int i, int32_t *uid, char *name, int n);
int pl_midi_connect(int i, int32_t uid); // OSStatus
void pl_midi_run(void);  // CFRunLoopRun on the calling thread
