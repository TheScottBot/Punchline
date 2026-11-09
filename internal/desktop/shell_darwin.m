// macOS menu bar shell: NSStatusItem + NSMenu, Carbon hot keys, a temporary
// key-capture panel for rebinding and IOKit USB change notifications.
// Compiled with ARC.
// All state lives on the main thread; the logic stays in Go.

#import <Cocoa/Cocoa.h>
#import <Carbon/Carbon.h>
#import <IOKit/IOKitLib.h>
#include "shell_darwin.h"
#include "_cgo_export.h"

@interface PLDelegate : NSObject <NSApplicationDelegate, NSMenuDelegate, NSWindowDelegate>
@end

static PLDelegate *gDelegate;
static NSStatusItem *gItem;
static NSMenu *gMenu;
static NSPanel *gPanel;
static NSTextField *gPanelText;
static id gMonitor;
static int gCapturing;
static IONotificationPortRef gUSBPort;

@implementation PLDelegate
- (void)menuNeedsUpdate:(NSMenu *)menu {
	if (menu != gMenu) return;
	[menu removeAllItems];
	goBuildMenu((__bridge void *)menu);
}
- (void)menuAction:(NSMenuItem *)item {
	goMenuAction((int)item.tag);
}
- (NSApplicationTerminateReply)applicationShouldTerminate:(NSApplication *)app {
	goTerminate(); // logout / restart / shutdown
	return NSTerminateNow;
}
- (void)windowDidResignKey:(NSNotification *)n {
	if (gCapturing) goCaptureCancel();
}
- (BOOL)windowShouldClose:(NSWindow *)w {
	if (gCapturing) goCaptureCancel();
	return YES;
}
@end

static OSStatus hotKeyHandler(EventHandlerCallRef next, EventRef ev, void *ud) {
	EventHotKeyID hk;
	if (GetEventParameter(ev, kEventParamDirectObject, typeEventHotKeyID, NULL, sizeof hk, NULL, &hk) == noErr)
		goHotkey(hk.id);
	return noErr;
}

void pl_app_init(void) {
	[NSApplication sharedApplication];
	[NSApp setActivationPolicy:NSApplicationActivationPolicyAccessory]; // no Dock icon, even unbundled
	gDelegate = [PLDelegate new];
	[NSApp setDelegate:gDelegate];

	gItem = [[NSStatusBar systemStatusBar] statusItemWithLength:NSSquareStatusItemLength];
	gMenu = [[NSMenu alloc] initWithTitle:@""];
	gMenu.autoenablesItems = NO;
	gMenu.delegate = gDelegate;
	gItem.menu = gMenu;

	EventTypeSpec spec = {kEventClassKeyboard, kEventHotKeyPressed};
	InstallApplicationEventHandler(NewEventHandlerUPP(hotKeyHandler), 1, &spec, NULL, NULL);

	gMonitor = [NSEvent addLocalMonitorForEventsMatchingMask:NSEventMaskKeyDown handler:^NSEvent *(NSEvent *e) {
		if (!gCapturing) return e;
		if (!e.isARepeat) goCaptureKey(e.keyCode);
		return nil; // swallowed
	}];
}

void pl_app_run(void) {
	[NSApp run];
}

void pl_app_stop(void) {
	[gMenu cancelTrackingWithoutAnimation];
	if (gItem) {
		[[NSStatusBar systemStatusBar] removeStatusItem:gItem];
		gItem = nil;
	}
	[NSApp stop:nil];
	// stop: takes effect after the next event; post one so run returns now.
	NSEvent *e = [NSEvent otherEventWithType:NSEventTypeApplicationDefined location:NSZeroPoint
	                           modifierFlags:0 timestamp:0 windowNumber:0 context:nil
	                                 subtype:0 data1:0 data2:0];
	[NSApp postEvent:e atStart:YES];
}

int pl_is_main(void) {
	return [NSThread isMainThread] ? 1 : 0;
}

// Work is queued on the main run loop in the common modes, so it also runs
// while the menu is open (event tracking mode), where the GCD main queue
// does not.
static void onMain(void (^block)(void)) {
	CFRunLoopRef rl = CFRunLoopGetMain();
	CFRunLoopPerformBlock(rl, kCFRunLoopCommonModes, block);
	CFRunLoopWakeUp(rl);
}

void pl_dispatch_main(uintptr_t h, int wait) {
	if (wait && [NSThread isMainThread]) {
		goMainCall(h);
		return;
	}
	if (!wait) {
		onMain(^{ goMainCall(h); });
		return;
	}
	dispatch_semaphore_t done = dispatch_semaphore_create(0);
	onMain(^{
		goMainCall(h);
		dispatch_semaphore_signal(done);
	});
	dispatch_semaphore_wait(done, DISPATCH_TIME_FOREVER);
}

void pl_after(double seconds, int token) {
	dispatch_after(dispatch_time(DISPATCH_TIME_NOW, (int64_t)(seconds * NSEC_PER_SEC)),
	               dispatch_get_global_queue(QOS_CLASS_USER_INTERACTIVE, 0), ^{
		onMain(^{ goTimer(token); });
	});
}

void pl_set_icon(const void *rgba, int px, double points, const char *tooltip) {
	if (!gItem) return;
	NSBitmapImageRep *rep = [[NSBitmapImageRep alloc]
		initWithBitmapDataPlanes:NULL pixelsWide:px pixelsHigh:px bitsPerSample:8 samplesPerPixel:4
		                hasAlpha:YES isPlanar:NO colorSpaceName:NSDeviceRGBColorSpace
		             bytesPerRow:px * 4 bitsPerPixel:32];
	memcpy(rep.bitmapData, rgba, (size_t)px * px * 4);
	rep.size = NSMakeSize(points, points);
	NSImage *img = [[NSImage alloc] initWithSize:NSMakeSize(points, points)];
	[img addRepresentation:rep];
	img.template = NO; // keep Thyme's colours
	gItem.button.image = img;
	gItem.button.toolTip = [NSString stringWithUTF8String:tooltip];
}

void pl_open_menu(void) {
	if (gItem) [gItem.button performClick:nil];
}

void *pl_menu_add(void *m, int kind, const char *text, int checked, int disabled, int tag) {
	NSMenu *menu = (__bridge NSMenu *)m;
	if (kind == 2) {
		[menu addItem:[NSMenuItem separatorItem]];
		return NULL;
	}
	NSMenuItem *it = [[NSMenuItem alloc] initWithTitle:[NSString stringWithUTF8String:text]
	                                            action:NULL keyEquivalent:@""];
	it.state = checked ? NSControlStateValueOn : NSControlStateValueOff;
	it.enabled = !disabled;
	if (kind == 1) {
		it.target = gDelegate;
		it.action = @selector(menuAction:);
		it.tag = tag;
	}
	[menu addItem:it];
	if (kind == 3) {
		NSMenu *sub = [[NSMenu alloc] initWithTitle:@""];
		sub.autoenablesItems = NO;
		it.submenu = sub;
		return (__bridge void *)sub; // owned by the item, which the menu owns
	}
	return NULL;
}

int pl_hotkey_register(int keycode, uint32_t id, void **ref) {
	EventHotKeyID hk = {'PNCH', id};
	EventHotKeyRef r = NULL;
	OSStatus st = RegisterEventHotKey((UInt32)keycode, 0, hk, GetApplicationEventTarget(),
	                                  kEventHotKeyExclusive, &r);
	*ref = r;
	return (int)st;
}

void pl_hotkey_unregister(void *ref) {
	if (ref) UnregisterEventHotKey((EventHotKeyRef)ref);
}

void pl_capture_show(const char *title, const char *text) {
	if (!gPanel) {
		gPanel = [[NSPanel alloc] initWithContentRect:NSMakeRect(0, 0, 400, 96)
		                                    styleMask:NSWindowStyleMaskTitled | NSWindowStyleMaskClosable
		                                      backing:NSBackingStoreBuffered defer:NO];
		gPanel.releasedWhenClosed = NO;
		gPanel.hidesOnDeactivate = NO;
		gPanel.level = NSFloatingWindowLevel;
		gPanel.delegate = gDelegate;
		gPanelText = [NSTextField wrappingLabelWithString:@""];
		gPanelText.alignment = NSTextAlignmentCenter;
		gPanelText.font = [NSFont systemFontOfSize:15];
		gPanelText.frame = NSInsetRect(gPanel.contentView.bounds, 20, 18);
		gPanelText.autoresizingMask = NSViewWidthSizable | NSViewHeightSizable;
		[gPanel.contentView addSubview:gPanelText];
	}
	gPanel.title = [NSString stringWithUTF8String:title];
	gPanelText.stringValue = [NSString stringWithUTF8String:text];
	[gPanel center];
	gCapturing = 1;
	[NSApp activate];
	[gPanel makeKeyAndOrderFront:nil];
}

void pl_capture_text(const char *text) {
	gCapturing = 0;
	if (gPanel) gPanelText.stringValue = [NSString stringWithUTF8String:text];
}

void pl_capture_close(void) {
	gCapturing = 0;
	if (gPanel && gPanel.visible) {
		[gPanel orderOut:nil];
		[NSApp hide:nil]; // hand focus back to the app the user was in
	}
}

void pl_open_path(const char *path) {
	[[NSWorkspace sharedWorkspace] openURL:[NSURL fileURLWithPath:[NSString stringWithUTF8String:path]]];
}

static void drain(io_iterator_t it) {
	io_object_t o;
	while ((o = IOIteratorNext(it))) IOObjectRelease(o);
}

static void usbChanged(void *ref, io_iterator_t it) {
	drain(it); // re-arms the notification
	goUSBChanged();
}

void pl_watch_usb(void) {
	if (gUSBPort) return;
	gUSBPort = IONotificationPortCreate(kIOMainPortDefault);
	CFRunLoopAddSource(CFRunLoopGetMain(), IONotificationPortGetRunLoopSource(gUSBPort), kCFRunLoopCommonModes);
	io_iterator_t added = 0, removed = 0;
	if (IOServiceAddMatchingNotification(gUSBPort, kIOFirstMatchNotification, IOServiceMatching("IOUSBHostDevice"),
	                                     usbChanged, NULL, &added) == KERN_SUCCESS)
		drain(added);
	if (IOServiceAddMatchingNotification(gUSBPort, kIOTerminatedNotification, IOServiceMatching("IOUSBHostDevice"),
	                                     usbChanged, NULL, &removed) == KERN_SUCCESS)
		drain(removed);
}
