#!/bin/sh
set -eu

cd "$(dirname "$0")"
PROJECT=$PWD
GOPATH=$PROJECT/.go
WORKSPACE=.go/src/bottle

# parse command arguments
PREFIX=/usr/local
SHOULD_INSTALL=false
SHOULD_UNINSTALL=false
while [ "${1:-""}" != "" ]; do
    case $1 in
        --prefix )
            shift
            PREFIX=$1
            ;;

        --install )
            SHOULD_INSTALL=true
            ;;

        --uninstall )
            SHOULD_UNINSTALL=true
            ;;
    esac
    shift
done

if [ "$SHOULD_UNINSTALL" = true ]; then
  rm "$PREFIX/bin/bottle"
  exit
fi

# setup golang workspace
mkdir -p $WORKSPACE
rsync -rum --delete --exclude ".*" src/ $WORKSPACE
if [ "$SHOULD_INSTALL" = false ]; then
  if [ -e .go/bin/bottle ]; then
    rm -f .go/bin/bottle
  fi
fi

# build golang binary
cd $WORKSPACE
GOPATH=$GOPATH go get -d ./...
GOPATH=$GOPATH go install .
cd $PROJECT

#  maybe install
if [ "$SHOULD_INSTALL" = true ]; then
  install -m 0755 .go/bin/bottle "$PREFIX/bin"
fi
