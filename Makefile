PREFIX ?= $(HOME)/.local/bin

.PHONY: build install clean

build:
	go build -o bin/skeleton .

install: build
	mkdir -p $(PREFIX)
	cp bin/skeleton $(PREFIX)/skeleton

clean:
	rm -rf bin/
