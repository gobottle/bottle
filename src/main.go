package main

import (
	"flag"
	"log"
	"os"
	"time"

	"bottle/debug"
	"bottle/shutil"
)

// TODO: Better panic error output (maybe defer shutil.RescueExit()?)

func init() {
	// TODO: Add as a "main" command-line flag? (possibly secret...)
	debug.ShouldTimeFunctions = false
}

func main() {
	defer debug.TimedFunction(time.Now(), "main()")

	// Configure logger
	log.SetFlags(0)

	// Configure cli flags
	flag.Usage = printHelp
	flag.Parse()

	// Parse the command
	var command string
	args := flag.Args()
	if len(args) > 0 {
		command = args[0]
		args = args[1:]
	} else {
		flag.Usage()
		shutil.Exit(0)
	}

	// Handle the chosen command
	switch command {
	case "build":
		var flags BuildFlags
		build := flag.NewFlagSet("build", flag.ExitOnError)
		build.Usage = printHelpBuild
		build.StringVar(&flags.outfile, "o", "", "")
		build.StringVar(&flags.outdir, "out-dir", "", "")
		build.StringVar(&flags.ldflags, "ldflags", "", "")
		build.Parse(args)
		if len(build.Args()) > 0 {
			shutil.Stderr("error: unexpected argument '" + build.Arg(0) + "'\n")
			shutil.Exit(1)
		}

		workdir := shutil.Pwd() // NOTE: changed by syncProject
		project := syncProject(workdir)
		buildProject(project, workdir, flags)
		shutil.Exit(0)

	case "exec":
		exec := flag.NewFlagSet("exec", flag.ExitOnError)
		exec.Usage = printHelpExec
		exec.Parse(args)
		args := exec.Args()
		if len(args) == 0 {
			shutil.Stderr("error: missing tool to execute\n")
			shutil.Exit(1)
		}

		workdir := shutil.Pwd() // NOTE: changed by syncProject
		project := syncProject(workdir)
		execTool(project, args[0], args[1:])
		shutil.Exit(0)

	case "publish":
		var flags PublishFlags
		publish := flag.NewFlagSet("publish", flag.ExitOnError)
		publish.Usage = printHelpPublish
		publish.StringVar(&flags.release, "release", "", "")
		publish.Parse(args)
		if len(publish.Args()) > 0 {
			shutil.Stderr("error: unexpected argument '" + publish.Arg(0) + "'\n")
			shutil.Exit(1)
		}

		workdir := shutil.Pwd() // NOTE: changed by syncProject
		project := syncProject(workdir)
		publishProject(project, flags)
		shutil.Exit(0)

	case "which":
		var printRoot bool
		which := flag.NewFlagSet("which", flag.ExitOnError)
		which.Usage = printHelpWhich
		which.BoolVar(&printRoot, "root", false, "")
		which.Parse(args)
		args := which.Args()
		if len(args) != 1 {
			shutil.Stderr("error: 'which' expects a single path\n")
			shutil.Exit(1)
		}

		// Check the target path
		targetPath := args[0]
		if !shutil.Exists(targetPath) {
			shutil.Stderr("error: the file '" + targetPath + "' does not exist\n")
			shutil.Exit(1)
		} else if !shutil.IsDirectory(targetPath) && !shutil.IsRegularFile(targetPath) {
			shutil.Stderr("error: the file '" + targetPath + "' is not a regular file or directory\n")
			shutil.Exit(1)
		}
		if shutil.IsRegularFile(targetPath) {
			targetPath = shutil.Dirname(targetPath)
		}

		// Find the config file
		shutil.Cd(targetPath)
		cfg, err := discoverPackage(".", "./Bottle.toml", true)
		if err != nil {
			shutil.Stderr("error: could not find Bottle.toml or GOPATH\n")
			shutil.Stderr(err.Error() + "\n")
			shutil.Exit(1)
		}

		// Output the project information
		message := "[" + cfg.Package.Name + "] "
		if printRoot {
			message += cfg.Package.Root
		} else {
			message += cfg.Project
		}
		shutil.Echo(message)
		shutil.Exit(0)

	case "help":
		if len(args) == 0 {
			printHelp()
			shutil.Exit(0)
		}

		switch args[0] {
		case "build":
			printHelpBuild()
		case "exec":
			printHelpExec()
		case "publish":
			printHelpPublish()
		case "which":
			printHelpWhich()
		default:
			shutil.Stderr("error: unknown help topic '" + command + "'\n")
			shutil.Exit(1)
		}
		shutil.Exit(0)

	default:
		shutil.Stderr("error: unknown command '" + command + "'\n")
		shutil.Stderr(commandDescriptions + "\n")
		shutil.Exit(1)
	}

	shutil.Stderr("error: reached end of main before 'Exit' (this is a bug!)")
	shutil.Exit(2) // catch-all
}

func syncProject(pwd string) *Config {
	defer debug.TimedFunction(time.Now(), "syncProject("+pwd+")")

	//  Read the config file
	cfg, err := discoverPackage(".", "./Bottle.toml", true)
	if err != nil {
		shutil.Stderr("error: could not find Bottle.toml or GOPATH\n")
		shutil.Stderr(err.Error() + "\n")
		shutil.Exit(1)
	}

	shutil.Cd(cfg.Project)
	os.Setenv("GOPATH", cfg.Workspace)

	// Discover, fetch, and install dependencies
	deps := NewDependencyTracker(cfg)
	err = deps.ResolveAll()
	if err != nil {
		log.Fatal(err)
	}
	err = deps.InstallAll()
	if err != nil {
		log.Fatal(err)
	}

	// Copy this project into the workspace
	pathResolver(cfg.Package.Root, cfg.Package.Name, cfg.Workspace)

	return cfg
}
