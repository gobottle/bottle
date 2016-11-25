.PHONY: all build install uninstall clean fmt

all: build

build:
	./bootstrap.sh

install:
	./bootstrap.sh --install

uninstall:
	./bootstrap.sh --uninstall

clean:
	rm -r .go

fmt:
	bottle exec go fmt ./...
