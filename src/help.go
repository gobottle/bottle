package main

import "bottle/shutil"

var commandDescriptions = `
Commands:
  build      Compile the current project
  exec       Execute a tool within the virtual GOPATH
  publish    Package and release the current project
  which      Find which project contains the target file`

func printHelp() {
	shutil.Echo(`A less assuming Go build tool

Usage:
  bottle <command> [<args>...]
  bottle [options]

Options:
  -h, --help
      Print this message (use "bottle help <command>" for more)
` + commandDescriptions)
}

func printHelpBuild() {
	shutil.Echo(`Compile the current project

Usage:
  bottle build [options]

Options:
  -h, --help
      Print this message
  -o string
      Write output to the specified file
  --out-dir string
      Write outputs into the a specified directory; ignored if -o is specified
  --ldflags string
      Arguments to pass on each "go tool link" invocation`)
}

func printHelpWhich() {
	shutil.Echo(`Find which project contains the target file

Usage:
bottle which [file]

Options:
  -h, --help
      Print this message
  --root
      Print the root package path instead of the project path

Example:
  [myproject] /path/to/project`)
}

func printHelpExec() {
	shutil.Echo(`Execute a tool within the virtual GOPATH

Usage:
  bottle exec [tool]

Options:
  -h, --help
      Print this message

Notes:
  This switches to the project's root package directory in the temporary Go
  workspace and executes a tool after setting GOPATH in the environment.

  All of the trailing arguments are passed to the executed tool.

  Any added or updated files are synchronized back to the source directory.`)
}

func printHelpPublish() {
	shutil.Echo(`Package and release the current project

Usage:
  bottle publish [options]

Options:
  -h, --help
      Print this message

Warning:
  This is currently SUPER special-cased for publishing "gobottle/bottle"
  specifically, and it probably won't be very useful for most projects.

  If you'd like to share some ideas about how to implement more-generic
  publishing, write up a detailed description in a GitHub issue.`)
}
