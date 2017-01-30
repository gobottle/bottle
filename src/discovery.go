package main

import (
	"fmt"
	"os"
	"path"
	"strconv"

	"bottle/shutil"
	"bottle/toml"
)

type Config struct {
	Workspace string `toml:"-"` // directory of GOPATH
	Project   string `toml:"-"` // directory of config file and/or project root

	Missing    bool   `toml:"-"` // whether the project contains a config file
	ImportPath string `toml:"-"` // set if Missing is true

	Package struct {
		Name    string // name of the project's root package (required)
		Version string // version of the current code in the directory tree

		// These fields are for documentation-purposes only
		Authors    []string
		Repository string
		License    string

		Publish bool // whether the package is intended to be published

		Exclude []string // exclude these files copying the package to the workspace
		Root    string   // the source directory of this project's root package, if not the project directory
	}

	Dependencies map[string]configDependency

	Bin []struct {
		Name string
		Path string
	}
}

type configDependency struct {
	Install bool // whether the package should be installed in the workspace

	Path string
	Git  string
	// TODO: Hg string
	// TODO: Svn string
}

func discoverPackage(mayberoot, maybecfg string, searchParents bool) (*Config, error) {
	pkgroot, cfgpath := mayberoot, maybecfg
	if searchParents {
		pkgroot, cfgpath = findNearestConfig(mayberoot, maybecfg)
	}
	if len(pkgroot) == 0 {
		return nil, fmt.Errorf(`discoverPackage: unknown package root for "%s"`, shutil.Abspath(mayberoot))
	}
	if !shutil.Exists(pkgroot) {
		return nil, fmt.Errorf(`discoverPackage: no such directory "%s"`, pkgroot)
	}

	env := shutil.Env()
	cfg := new(Config)

	// Parse config file from "Bottle.toml"
	if len(cfgpath) > 0 && shutil.Exists(cfgpath) {
		err := toml.Unmarshal(shutil.Binread(cfgpath), cfg)
		if err != nil {
			return nil, err
		}
	} else {
		cfg.Missing = true
		cfg.ImportPath = shutil.Relpath(shutil.Path(env["GOPATH"], "src"), shutil.Abspath(pkgroot))
		cfg.Package.Name = path.Base(cfg.ImportPath)
	}

	// Set directories to absolute paths
	cfg.Project = shutil.Abspath(pkgroot)
	cfg.Workspace = shutil.Path(os.TempDir(), "bottle", cfg.Package.Name)
	if len(cfg.Package.Root) > 0 {
		// TODO: enforce that the package root is a subdirectory of "cfg.Project"
		cfg.Package.Root = shutil.Abspath(shutil.Path(pkgroot, cfg.Package.Root))
	} else {
		cfg.Package.Root = cfg.Project
	}

	return cfg, nil
}

// findNearestConfig file searches for a Bottle.toml config file by checking
// the current directory and then each parent directory in the path.
func findNearestConfig(mayberoot, maybecfg string) (string, string) {
	origRoot := mayberoot

loop:
	for i := 0; i < 255; i++ {
		if i > 100 {
			shutil.Stderr("error: infinite loop?", strconv.Itoa(i), mayberoot, maybecfg)
			shutil.Exit(1)
		}

		switch {
		case shutil.Exists(maybecfg):
			break loop
		case shutil.Abspath(mayberoot) != "/":
			mayberoot += "/.."
			maybecfg = mayberoot + "/Bottle.toml"
		default:
			env := shutil.Env()
			if len(env["GOPATH"]) > 0 && shutil.IsSubdir(origRoot, env["GOPATH"]) {
				return origRoot, ""
			} else {
				return "", ""
			}
		}
	}

	return mayberoot, maybecfg
}
