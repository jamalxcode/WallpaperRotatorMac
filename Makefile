APP     := WallpaperChanger.app
BIN     := $(APP)/Contents/MacOS/WallpaperChanger
ICNS    := $(APP)/Contents/Resources/AppIcon.icns
DEST    := /Applications

.PHONY: app install run once clean

app: $(ICNS)
	mkdir -p $(APP)/Contents/MacOS
	go build -o $(BIN) .
	cp Info.plist $(APP)/Contents/Info.plist
	codesign --force --sign - $(APP)

# Build the macOS icon set from the 1024px master in assets/icon.png
$(ICNS): assets/icon.png
	rm -rf build/AppIcon.iconset && mkdir -p build/AppIcon.iconset $(dir $(ICNS))
	for s in 16 32 128 256 512; do \
		sips -z $$s $$s assets/icon.png --out build/AppIcon.iconset/icon_$${s}x$${s}.png >/dev/null; \
		sips -z $$((s*2)) $$((s*2)) assets/icon.png --out build/AppIcon.iconset/icon_$${s}x$${s}@2x.png >/dev/null; \
	done
	iconutil -c icns build/AppIcon.iconset -o $(ICNS)

install: app
	rm -rf $(DEST)/$(APP)
	cp -R $(APP) $(DEST)/
	touch $(DEST)/$(APP)   # nudge Finder/Dock to refresh the icon
	open $(DEST)/$(APP)

run: app
	open $(APP)

once:
	go run . --once

clean:
	rm -rf $(APP) build wallpaper-changer
