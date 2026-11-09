// Native half of the macOS shell (shell_darwin.m). Every function must be
// called on the main thread unless noted.
#include <stdint.h>

void pl_app_init(void);
void pl_app_run(void);
void pl_app_stop(void);

int pl_is_main(void); // any thread
// pl_dispatch_main runs goMainCall(h) on the main thread; any thread. With
// wait set it returns after the call (runs inline when already on main).
void pl_dispatch_main(uintptr_t h, int wait);
void pl_after(double seconds, int token); // calls goTimer(token) on main

void pl_set_icon(const void *rgbaPremultiplied, int px, double points, const char *tooltip);
void pl_open_menu(void);
// Kinds match desktop.ItemKind: 0 label, 1 command, 2 separator, 3 submenu.
// Returns the new submenu for kind 3, otherwise NULL.
void *pl_menu_add(void *menu, int kind, const char *text, int checked, int disabled, int tag);

int pl_hotkey_register(int keycode, uint32_t id, void **ref); // OSStatus
void pl_hotkey_unregister(void *ref);

void pl_capture_show(const char *title, const char *text);
void pl_capture_text(const char *text); // ends capture, keeps the panel up
void pl_capture_close(void);

void pl_open_path(const char *path);
void pl_watch_usb(void);
