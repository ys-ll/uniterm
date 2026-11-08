//go:build darwin

package main

/*
#cgo CFLAGS: -x objective-c
#cgo LDFLAGS: -framework Cocoa

#import <Cocoa/Cocoa.h>
#import <objc/runtime.h>

// Set once the empty title bar accessory used to make room for a centred button
// row has been installed, so repeated show/resize calls do not stack copies.
static BOOL gTitlebarGrown = NO;

// Height (in points) of the header band where window drags run through the web
// view, and the original NSCursor -set implementation used by the drag-cursor
// guard below.
static double gHeaderBandHeight = 0.0;
static BOOL gCursorGuardInstalled = NO;
static void (*gOrigCursorSet)(id, SEL) = NULL;

static void uniMoveGroupTrackingArea(NSView* theme, NSRect target);

// uniPointerInHeaderBand reports whether the pointer is inside the window's
// header band, the part the web view owns and therefore drags through WebKit.
static BOOL uniPointerInHeaderBand(void) {
	NSWindow* win = [NSApp keyWindow];
	if (win == nil || gHeaderBandHeight <= 0.0) {
		return NO;
	}
	NSPoint point = [NSEvent mouseLocation];
	NSRect frame = [win frame];
	if (point.x < NSMinX(frame) || point.x > NSMaxX(frame)) {
		return NO;
	}
	double topInset = frame.size.height - (point.y - frame.origin.y);
	return topInset >= 0.0 && topInset <= gHeaderBandHeight;
}

// uniCursorSetGuard keeps the plain arrow cursor while the window is being
// dragged from the header band. Those drags start in the web view, and WebKit
// answers the mouse-down by pushing an I-beam cursor -- a text caret shown while
// the whole window moves. Any other cursor request (text selection in the
// terminal, resize handles, ...) is left untouched.
static void uniCursorSetGuard(id self, SEL _cmd) {
	if (self != [NSCursor arrowCursor] && ([NSEvent pressedMouseButtons] & 1) != 0 &&
		uniPointerInHeaderBand()) {
		gOrigCursorSet([NSCursor arrowCursor], _cmd);
		return;
	}
	gOrigCursorSet(self, _cmd);
}

static void uniInstallCursorGuard(void) {
	if (gCursorGuardInstalled) {
		return;
	}
	gCursorGuardInstalled = YES;
	Method method = class_getInstanceMethod([NSCursor class], @selector(set));
	gOrigCursorSet = (void (*)(id, SEL))method_getImplementation(method);
	method_setImplementation(method, (IMP)uniCursorSetGuard);
}

// uniPlaceTrafficLightsOnMain moves the native title bar so its buttons line up
// with the web header: a stock 20pt leading inset and vertically centred in the
// header row.
//
// Must run on the main thread: AppKit raises NSInternalInconsistencyException
// ("NSWindow drag regions should only be invalidated on the Main Thread!") when
// a title-bar widget's frame changes from any other thread, and Wails delivers
// window events on a Go goroutine.
static void uniPlaceTrafficLightsOnMain(void* handle, double leftInset, double headerHeight) {
	@autoreleasepool {
		NSWindow* win = (NSWindow*)handle;
		if (win == nil) {
			return;
		}
		NSButton* close = [win standardWindowButton:NSWindowCloseButton];
		if (close == nil) {
			return;
		}
		NSView* titlebar = [close superview];
		if (titlebar == nil) {
			return;
		}

		// Keep the buttons in their own title bar view so AppKit keeps owning
		// their hover, tooltip and drag tracking. To have room for a lower,
		// centred position the bar itself is grown to the header height first.
		double barHeight = [titlebar bounds].size.height;
		if (!gTitlebarGrown && barHeight < headerHeight - 0.5) {
			NSView* spacer = [[NSView alloc] initWithFrame:NSMakeRect(0, 0, 0, headerHeight - barHeight)];
			NSTitlebarAccessoryViewController* accessory = [[NSTitlebarAccessoryViewController alloc] init];
			[accessory setView:spacer];
			[accessory setLayoutAttribute:NSLayoutAttributeBottom];
			[win addTitlebarAccessoryViewController:accessory];
			gTitlebarGrown = YES;
			barHeight = [titlebar bounds].size.height;
		}

		NSButton* buttons[3];
		buttons[0] = close;
		buttons[1] = [win standardWindowButton:NSWindowMiniaturizeButton];
		buttons[2] = [win standardWindowButton:NSWindowZoomButton];

		double centreY = [titlebar isFlipped]
			? headerHeight / 2.0
			: (barHeight - headerHeight / 2.0);

		for (int i = 0; i < 3; i++) {
			NSButton* button = buttons[i];
			if (button == nil) {
				continue;
			}
			NSSize size = [button frame].size;
			[button setFrameOrigin:NSMakePoint(leftInset + 20.0 * i, centreY - size.height / 2.0)];
		}
		// Tracking areas are rebuilt from the buttons' current frames, so let the
		// title bar lay out once, then re-apply the positions in case its own
		// layout pass reset them.
		[titlebar setNeedsLayout:YES];
		[titlebar layoutSubtreeIfNeeded];
		for (int i = 0; i < 3; i++) {
			NSButton* button = buttons[i];
			if (button == nil) {
				continue;
			}
			NSSize size = [button frame].size;
			[button setFrameOrigin:NSMakePoint(leftInset + 20.0 * i, centreY - size.height / 2.0)];
			[button updateTrackingAreas];
		}
		[titlebar updateTrackingAreas];
		// One more layout pass with the buttons already at their final spot: the
		// title bar caches the traffic-light hover region during layout, so this
		// is what moves that region along with the buttons.
		[titlebar setNeedsLayout:YES];
		[titlebar layoutSubtreeIfNeeded];

		// The window's own frame view caches the group-hover rectangle it uses to
		// reveal the button symbols; lay it out too and then re-home that rectangle
		// onto the re-framed buttons.
		NSView* theme = [[win contentView] superview];
		if (theme != nil) {
			[theme setNeedsLayout:YES];
			[theme layoutSubtreeIfNeeded];
			[theme updateTrackingAreas];
			NSRect group = NSMakeRect(leftInset, centreY - 8.0, 2.0 * 20.0 + 14.0, 16.0);
			uniMoveGroupTrackingArea(theme, [titlebar convertRect:group toView:theme]);
		}

		// Keep the plain arrow cursor while the window is dragged by the header
		// band that the web view owns.
		gHeaderBandHeight = headerHeight;
		uniInstallCursorGuard();
	}
}

// uniMoveGroupTrackingArea re-homes the private tracking area AppKit installs on
// the window's frame view to reveal the traffic-light symbols on hover.
//
// AppKit creates it once around the stock button row (7pt inset, 16pt tall,
// ~54pt wide, options 0x1081) and never recomputes it, so after the buttons are
// re-framed the symbols still pop up at the old corner. The area is removed and
// re-added at the buttons' new union rect, copying owner/options/userInfo so
// AppKit's own hover handling keeps working.
static void uniMoveGroupTrackingArea(NSView* theme, NSRect target) {
	NSArray* areas = [[theme trackingAreas] copy];
	for (NSTrackingArea* area in areas) {
		if ([area options] != 4225) {
			continue;
		}
		NSRect rect = [area rect];
		if (fabs(rect.size.height - 16.0) > 1.0 || rect.size.width > 100.0) {
			continue;
		}
		if (NSEqualRects(rect, target)) {
			continue;
		}
		id owner = [area owner];
		NSDictionary* userInfo = [area userInfo];
		[theme removeTrackingArea:area];
		NSTrackingArea* moved = [[NSTrackingArea alloc] initWithRect:target
			options:[area options] owner:owner userInfo:userInfo];
		[theme addTrackingArea:moved];
	}
}

void uniPlaceTrafficLights(void* handle, double leftInset, double headerHeight) {
	if ([NSThread isMainThread]) {
		uniPlaceTrafficLightsOnMain(handle, leftInset, headerHeight);
		return;
	}
	dispatch_async(dispatch_get_main_queue(), ^{
		uniPlaceTrafficLightsOnMain(handle, leftInset, headerHeight);
	});
}
*/
import "C"

import "unsafe"

const (
	// macHeaderHeight is the header row height in points: .app-header is
	// 2.75rem tall and html.mac sets the root font-size to 18.6667px, so
	// 2.75rem == 51.33px.
	macHeaderHeight = 51.33
	// macTrafficLightInset is where the first (close) button starts, matching
	// the 20pt leading inset AppKit uses in stock titled windows.
	macTrafficLightInset = 20.0
)

// applyMacTrafficLightLayout lines the native window buttons up with the web
// header. Called after every show and resize, because AppKit re-lays out the
// title bar on those events.
func applyMacTrafficLightLayout(nativeWindow unsafe.Pointer) {
	if nativeWindow == nil {
		return
	}
	C.uniPlaceTrafficLights(nativeWindow, C.double(macTrafficLightInset),
		C.double(macHeaderHeight))
}
