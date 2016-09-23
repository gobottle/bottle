***********************************
Bottle: an unassuming Go build tool
***********************************

Bottle is a Go build tool that doesn't force your dependencies to be at well-
known in-flexible filepaths (like in a Go workspace, or the vendor/ directory).

Bottle treats each directory as one package (library or executable), and puts
dependencies and project information into the `Bottle.toml` config file.

.. image:: https://img.shields.io/badge/release-alpha-orange.svg


Features
========

- Packages no longer need to be in a Go workspace (ie. src/pkg/bin)
- Packages no longer need to belong to a "remote" (they can be referenced by path)
- By default, subpackages (subdirectories), can use a relative path from the
  project root.  This allows the repository host to change without requiring
  all of the project's imports to be renamed.
- Backwards compatible with existing repositories (use GOPATH if you want).


Installation
============

To use gobottle, first you need to install Go and ensure it is on your path.
You don't need to set up a GOPATH, but you can, if you want.

From the source directory::

    make && sudo make install

To install Atom text editor integrations::

    cd atom && npm install && apm link


License
=======

Bottle is licensed under a BSD 3-Clause license (see LICENSE_).

.. _LICENSE: LICENSE
