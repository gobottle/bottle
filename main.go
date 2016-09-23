package main

import (
	"bufio"
	"flag"
	"io"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"bottle/filemutex"
	sh "bottle/shutil"

	"encoding/toml"
)

type Config struct {
	Dir     string `toml:"-"` // set automatically (not in config)
	Missing bool   `toml:"-"`

	Package struct {
		Name       string
		Version    string
		Authors    []string
		Repository string
		License    string
		Exclude    []string
		Publish    bool
	}

	Dependencies map[string]struct {
		Install bool // if package should be installed (so its command can be used)

		Import string // uses Go's default toolchain to find and fetch this package
		Path   string

		// TODO
		/*
			Git string
			Hg string
			Svn string
		*/
	}

	Bin []struct {
		Name string
		Path string
	}
}

func main() {
	// TODO: Better panic error output (maybe defer sh.RescueExit()?)

	// NOTE: paths in arguments for tool must be relative to Bottle.toml (or GOPATH if no build.toml)
	// NOTE: changes made to files should be rsync'd back to the source with their imports re-renamed

	var which, mustGenerate bool
	var tool, ldflags, outfile, outdir string
	flag.StringVar(&outfile, "o", "", "write output to the specified file; ignored if multiple outputs are specified")
	flag.StringVar(&outdir, "out-dir", "", "write outputs into the a specified directory; ignored if -o is specified")
	flag.StringVar(&ldflags, "ldflags", "", "arguments to pass on each \"go tool link\" invocation")
	flag.StringVar(&tool, "tool", "", "run a tool that relies on GOPATH")
	// HACK: This CLI flag is a temporary hack; must review the use cases for generate later.
	//   --generate is a workaround for using rsync to delete old removed source files
	flag.BoolVar(&mustGenerate, "generate", false, "run \"go generate\" before running the tool specified by --tool")
	flag.BoolVar(&which, "which", false, "print the directory of Bottle.toml for the file")

	flag.Usage = func() {
		sh.Echo(`Usage of bottle:
  -o string
    	write output to the specified file; ignored if multiple outputs are specified
  --out-dir string
    	write outputs into the a specified directory; ignored if -o is specified
  --ldflags string
    	arguments to pass on each "go tool link" invocation
  --tool string
    	run a tool that relies on GOPATH
  --generate
    	run "go generate" before running the tool specified by --tool
  --which
    	print the name and directory containing Bottle.toml for this package`)
	}

	flag.Parse()
	env := sh.Env()
	pwd := sh.Pwd()

	/*  Read the config file  */
	var cfg Config
	cfg.Package.Publish = true // set default
	{
		// find the closest config file (recurse on parents)
		var pkgroot, cfgpath string
		args := flag.Args()
		if len(args) > 1 {
			sh.Stderr(": too many positional arguments (max 1)")
			sh.Exit(1)
		} else if len(args) == 1 {
			path := args[0]
			if sh.IsDirectory(path) {
				pkgroot = path
			} else if sh.IsRegularFile(path) {
				pkgroot = sh.Dirname(path)
			} else if sh.Exists(path) {
				sh.Stderr("bottle: the file '" + path + "' is not a regular file or directory\n")
				sh.Exit(1)
			} else {
				sh.Stderr("bottle: the file/directory '" + path + "' does not exist\n")
				sh.Exit(1)
			}
			cfgpath = sh.Path(pkgroot, "Bottle.toml")
		} else {
			pkgroot = "."
			cfgpath = "./Bottle.toml"
		}

		mayberoot := pkgroot
		maybecfg := cfgpath
	loop:
		for {
			switch {
			case sh.Exists(maybecfg):
				break loop
			case sh.Abspath(mayberoot) != "/":
				mayberoot += "/.."
				maybecfg = mayberoot + "/Bottle.toml"
			default:
				if len(env["GOPATH"]) > 0 && sh.IsSubdir(pkgroot, env["GOPATH"]) {
					mayberoot = pkgroot
					maybecfg = ""
					break loop
				}

				sh.Stderr("bottle: could not find Bottle.toml or GOPATH\n")
				sh.Exit(1)
			}
		}

		cfgpath = maybecfg
		pkgroot = mayberoot
		if len(cfgpath) > 0 {
			sh.Must(toml.Unmarshal(sh.Binread(cfgpath), &cfg))
		} else {
			cfg.Missing = true
			cfg.Package.Name = sh.Relpath(env["GOPATH"], sh.Abspath(pkgroot))
		}
		cfg.Dir = sh.Abspath(pkgroot)
		sh.Cd(cfg.Dir)
	}

	/* Handle the "which" cli flag */
	if which {
		sh.Echo("[" + cfg.Package.Name + "] " + cfg.Dir)
		sh.Exit(0)
	}

	/* Build map of name to path for import renaming */
	renames := make(map[string]string, len(cfg.Dependencies))
	backnames := make(map[string]string, len(cfg.Dependencies))
	for name, dep := range cfg.Dependencies {
		if len(dep.Import) > 0 {
			renames[name] = dep.Import
			backnames[dep.Import] = name
		}
	}

	var workspace, pkgpath string
	if !cfg.Missing {
		/*  Make a temporary go workspace (TODO: support clean builds with sh.RmRecursive`)  */
		workspace = "/tmp/bottle-" + cfg.Package.Name
		pkgpath = workspace + "/src/" + cfg.Package.Name
		sh.MkdirParents(pkgpath, 0755)
		defer func() { // TODO: Replace with `sh.OnFatal(func() { sh.RmRecursive(workspace) })`
			if err := recover(); err != nil {
				// cleanup if we panic, since we might be in an inconsistent state
				sh.RmRecursive(workspace)
				panic(err) // TODO: preserve stack trace
			}
		}()
		os.Unsetenv("GOPATH")
		os.Setenv("GOPATH", workspace)

		/*  Lock the workspace before screwing around with it  */
		acquiredLock := make(chan bool)
		workspaceMutex, err := filemutex.New(sh.Path(workspace, ".workspace.lock"))
		{
			if err != nil {
				sh.Stderr("bottle: " + err.Error())
			}
			go func() {
				workspaceMutex.Lock()
				acquiredLock <- true
			}()

			select {
			case <-time.After(3 * time.Second):
				sh.Stderr("bottle: timed-out waiting for lock on directory")
				sh.Exit(1)
				// TODO: try to delete lock file if it is very old
			case <-acquiredLock:
				defer workspaceMutex.Unlock() // safe to continue
			}
		}

		/*  Copy the project to the golang workspace  */
		var buildChanged, lockChanged bool
		changes := syncFiles(cfg.Dir, pkgpath, syncOptions{Delete: true})
		for _, path := range changes {
			if strings.HasSuffix(path, "/Bottle.toml") {
				buildChanged = true
			} else if strings.HasSuffix(path, "/Build.lock") {
				lockChanged = true
			}
		}

		/*  Rename the imports at the destination  */
		renameImports(changes, renames)

		/*  Fetch any local dependencies  */
		for name, dep := range cfg.Dependencies {
			if len(dep.Path) > 0 {
				deppath := path.Join(workspace, "src", name)

				// FIXME: Recursively fetch and rename imports
				depChanges := syncFiles(
					sh.Abspath(dep.Path),
					sh.Path(workspace, "src", name),
					syncOptions{Delete: true})

				// HACK: Assume the Bottle.toml is in the target directory and apply renames
				var depCfg Config
				cfgpath := sh.Path(dep.Path, "Bottle.toml")
				if sh.Exists(cfgpath) {
					sh.Must(toml.Unmarshal(sh.Binread(cfgpath), &depCfg))
				}

				depRenames := make(map[string]string, len(cfg.Dependencies))
				for name, dep := range depCfg.Dependencies {
					if len(dep.Import) > 0 {
						depRenames[name] = dep.Import
					}
				}

				renameImports(depChanges, depRenames)

				// HACK: Be more careful about get-ing the dependency's dependencies
				{
					args := []string{`get`, `-d`, `./...`}
					if sh.Exists(sh.Path(deppath, "vendor")) {
						args[2] = `.` // if the vendor directory exists, we can't use "./..."
					}

					cmd := sh.Cmd(`go`, args...)
					cmd.Env = append(cmd.Env, "GOPATH="+workspace)
					cmd.Dir = deppath
					output, err := cmd.Try()
					if err != nil {
						sh.Stderr(output)
						sh.Exit(1)
					}
				}

				if dep.Install {
					// HACK: This was copied'n'pasted from below; do the proper thing and break it out
					cmd := sh.Cmd(`go`, `install`, `.`)
					cmd.Env = append(cmd.Env, "GOPATH="+workspace)
					cmd.Dir = deppath
					output, err := cmd.Try()
					output = strings.Replace(output, deppath, ".", -1)
					if err != nil {
						sh.Stderr(output)
						sh.Exit(1)
					}
				}
			}
		}

		/*  Fetch any remote dependencies  */
		if buildChanged || lockChanged {
			args := []string{`get`, `-d`, `./...`}
			if sh.Exists(sh.Path(pkgpath, "vendor")) {
				args[2] = `.` // if the vendor directory exists, we can't use "./..."
			}

			cmd := sh.Cmd(`go`, args...)
			cmd.Env = append(cmd.Env, "GOPATH="+workspace)
			cmd.Dir = pkgpath
			output, err := cmd.Try()
			if err != nil {
				sh.Stderr(output)
				sh.Exit(1)
			}

			/* Install remote dependencies as necessary */
			for name, dep := range cfg.Dependencies {
				// TODO: Improve detection of implicit and explicit imports
				if len(dep.Path) == 0 && dep.Install {
					installPkgs := name + "/..."
					if len(dep.Import) > 0 {
						installPkgs = dep.Import + "/..."
					}

					cmd := sh.Cmd(`go`, `get`, installPkgs)
					cmd.Env = append(cmd.Env, "GOPATH="+workspace)
					cmd.Dir = workspace
					output, err := cmd.Try()
					if err != nil {
						sh.Stderr(output)
						sh.Exit(1)
					}
				}
			}
		}
	} else {
		workspace = env["GOPATH"]
		pkgpath = cfg.Dir
	}

	if len(tool) > 0 {

		/* Run generate, if it was requested */
		if mustGenerate {
			args := []string{`generate`, `./...`}
			if sh.Exists(sh.Path(pkgpath, "vendor")) {
				args = []string{`generate`, `.`}
			}

			cmd := sh.Cmd(`go`, args...)
			cmd.Env = append([]string{"PATH=" + workspace + "/bin:" + env["PATH"]}, cmd.Env...)
			cmd.Env = append(cmd.Env, "GOPATH="+workspace)
			cmd.Dir = pkgpath
			output, err := cmd.Try()
			output = strings.Replace(output, pkgpath, ".", -1)
			if err != nil {
				sh.Stderr(output)
				sh.Exit(1)
			}
		}

		/*  Run the tool  */
		shellsplit := regexp.MustCompile(`"[^"]*"|\'[^\']*\'|[^"\'\s]+`).FindAllString(tool, -1)
		if len(shellsplit) == 0 {
			sh.Stderr("bottle: unable to split the tool's name and arguments")
			sh.Exit(1)
		}

		cmd := sh.Cmd(shellsplit[0], shellsplit[1:]...)
		cmd.Env = append(cmd.Env, "GOPATH="+workspace)
		cmd.Dir = pkgpath
		cmd.Stdin = os.Stdin
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		err := cmd.Bind()
		if err != nil {
			sh.Exit(1)
		}

		if !cfg.Missing {
			// rename the imports for any changed files in the temp directory
			// (such that anything watching the project's filesystem doesn't trigger)
			toolChanges := syncFiles(pkgpath, cfg.Dir, syncOptions{Dryrun: true})
			renameImports(toolChanges, backnames)

			// sync only the files that have changed
			syncFiles(pkgpath, cfg.Dir, syncOptions{})

			// restore the workspace's imports for later builds
			renameImports(toolChanges, renames)
		}

	} else {

		/*  Build the project  */
		genArgs := []string{`generate`}  // `./...`}
		buildArgs := []string{`install`} // `./...`}
		if len(ldflags) > 0 {
			buildArgs = append(buildArgs, "-ldflags", ldflags)
		}

		// use the "./..." (build all) target unless a "vendor" directory exists
		if sh.Exists(sh.Path(pkgpath, "vendor")) {
			genArgs = append(genArgs, `.`)
			buildArgs = append(buildArgs, `.`)
		} else {
			genArgs = append(genArgs, `./...`)
			buildArgs = append(buildArgs, `./...`)
		}

		{
			cmd := sh.Cmd(`go`, genArgs...)
			cmd.Env = append([]string{"PATH=" + workspace + "/bin:" + env["PATH"]}, cmd.Env...)
			cmd.Env = append(cmd.Env, "GOPATH="+workspace)
			cmd.Dir = pkgpath
			output, err := cmd.Try()
			output = strings.Replace(output, pkgpath, ".", -1)
			if err != nil {
				sh.Stderr(output)
				sh.Exit(1)
			}
		}

		{
			cmd := sh.Cmd(`go`, buildArgs...)
			cmd.Env = append(cmd.Env, "GOPATH="+workspace)
			cmd.Dir = pkgpath
			output, err := cmd.Try()
			output = strings.Replace(output, pkgpath, ".", -1)
			if err != nil {
				sh.Stderr(output)
				sh.Exit(1)
			}
		}

		/* Copy the outputs to the output file/directory */
		binprefix := workspace + "/bin/"
		if len(outfile) > 0 {
			if !filepath.IsAbs(outfile) {
				outfile = sh.Path(pwd, outfile)
			}

			outdir := path.Dir(outfile)
			if !sh.Exists(outdir) {
				sh.MkdirParents(outdir, 0755)
			}

			if len(cfg.Bin) > 0 {
				bincfg := cfg.Bin[0]
				bindir := path.Dir(bincfg.Path)
				if len(bindir) > 0 && bindir != "." {
					sh.Cp(binprefix+path.Base(bindir), outfile)
				} else {
					sh.Cp(binprefix+bincfg.Name, outfile)
				}
			} else if sh.IsRegularFile(workspace + "/bin/" + cfg.Package.Name) {
				sh.Cp(binprefix+cfg.Package.Name, outfile)
			} else {
				sh.Stderr("bottle: no output exists that can be written to a file")
				sh.Exit(1)
			}
		} else if len(outdir) > 0 {
			if !filepath.IsAbs(outdir) {
				outdir = sh.Path(pwd, outdir)
			}

			if !sh.Exists(outdir) {
				sh.MkdirParents(outdir, 0755)
			}

			for _, bincfg := range cfg.Bin {
				bindir := path.Dir(bincfg.Path)
				if len(bindir) > 0 && bindir != "." {
					sh.Cp(binprefix+path.Base(bindir), sh.Path(outdir, bincfg.Name))
				} else {
					sh.Cp(binprefix+bincfg.Name, sh.Path(outdir, bincfg.Name))
				}
			}
		}
	}
}

type syncOptions struct {
	Dryrun bool
	Delete bool
}

func syncFiles(src, dest string, opts syncOptions) []string {
	// TODO: maybe implement file sync in Go
	// FIXME: try not to backwards sync files which have only had their imports renamed
	args := []string{`-rum`, `--exclude`, `.*`, `--exclude`, `*.orig`, `--out-format=/%f`}
	if opts.Delete {
		args = append(args, `--delete`)
	}
	if opts.Dryrun {
		args = append(args, `--dry-run`)
	}
	args = append(args, src+"/", dest)

	output := sh.Cmd(`rsync`, args...).Run()
	paths := strings.Split(strings.TrimSpace(output), "\n")
	if len(paths) == 1 && len(paths[0]) == 0 {
		paths = nil
	}

	kept := 0
	for i := range paths {
		path := paths[i]
		if strings.HasPrefix(path, "deleting ") {
			continue
		}

		if opts.Dryrun {
			paths[kept] = path
		} else {
			paths[kept] = sh.Path(dest, sh.Relpath(src, path))
		}
		kept++
	}
	return paths[:kept]
}

func renameImports(files []string, imports map[string]string) {
	var sources []string
	for _, file := range files {
		if strings.HasSuffix(file, ".go") {
			sources = append(sources, file)
		}
	}

	search := make([]string, 2*len(imports))
	for old, new := range imports {
		// TODO: is the number of replaces maybe becoming inefficient? maybe use gorename equiv
		search = append(search, "import \""+old, "import \""+new)
		search = append(search, "import . \""+old, "import . \""+new)
		search = append(search, "import _ \""+old, "import _ \""+new)
		search = append(search, "\t\""+old, "\t\""+new)
		search = append(search, "\t. \""+old, "\t. \""+new)
		search = append(search, "\t_ \""+old, "\t_ \""+new)
	}

	replacer := strings.NewReplacer(search...)
	for _, filename := range sources {
		replaceInFile(filename, replacer)
	}
}

func replaceInFile(filename string, replacer *strings.Replacer) {
	restore := true
	original := filename + ".orig"
	sh.Mv(filename, original)
	defer func() {
		if restore {
			sh.Mv(original, filename)
		} else {
			sh.Rm(original)
		}
	}()

	// open input and output files
	dest, err := os.Create(filename)
	if err != nil {
		panic(err)
	}
	defer dest.Close()
	src, err := os.Open(original)
	if err != nil {
		panic(err)
	}
	defer src.Close()

	// streaming replace imports
	scanner := bufio.NewReader(src)
	line, isPrefix, err := scanner.ReadLine()
	for err == nil {
		text := string(line)
		dest.WriteString(replacer.Replace(text))
		if !isPrefix {
			dest.Write([]byte{'\n'})
		}

		if strings.HasPrefix(text, "func ") ||
			strings.HasPrefix(text, "type ") ||
			strings.HasPrefix(text, "const ") ||
			strings.HasPrefix(text, "var ") {

			// import declarations must occur before all other decls,
			// so we can quit as soon as we find one
			break

		}

		line, isPrefix, err = scanner.ReadLine()
	}
	if err != nil && err != io.EOF {
		panic(err)
	}

	// write out the remainder of the file
	scanner.WriteTo(dest)

	// success!
	restore = false
	dest.Sync()
}
