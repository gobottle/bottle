.PHONY: all build install uninstall clean

all: build

build:
	./bootstrap.sh

install:
	./bootstrap.sh --install

uninstall:
	./bootstrap.sh --uninstall

clean:
	rm -r .go
