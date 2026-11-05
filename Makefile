APP     := WallpaperChanger.app
BIN     := $(APP)/Contents/MacOS/WallpaperChanger
DEST    := /Applications

.PHONY: app install run once clean

app:
	mkdir -p $(APP)/Contents/MacOS
	go build -o $(BIN) .
	cp Info.plist $(APP)/Contents/Info.plist
	codesign --force --sign - $(APP)

install: app
	rm -rf $(DEST)/$(APP)
	cp -R $(APP) $(DEST)/
	open $(DEST)/$(APP)

run: app
	open $(APP)

once:
	go run . --once

clean:
	rm -rf $(APP) wallpaper-changer
