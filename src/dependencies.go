package main

import (
	"fmt"
	"log"
	"path"
	"strings"
	"time"

	"bottle/debug"
	"bottle/shutil"
)

type DependencyTracker struct {
	rootConfig *Config

	usedImports    map[string]bool
	updatedImports map[string]bool // excludes "AlreadyResolved" dependencies
	canonicalPaths map[Dependency]string

	installPackages map[string]bool
	packagePrefixes map[string]string //  (eg. github.com/a/b/cmd/x to github.com/a/b)

	resolved   map[Dependency]bool
	unresolved []Dependency

	needsFallback []string
}

type Dependency struct {
	Protocol   string // "fallback", "go-get", "path", "git" (TODO: "hg", "bzr", "svn")
	Repository string
}

func NewDependencyTracker(cfg *Config) *DependencyTracker {
	deps := &DependencyTracker{
		usedImports:    make(map[string]bool),
		updatedImports: make(map[string]bool),
		canonicalPaths: make(map[Dependency]string),

		installPackages: make(map[string]bool),
		packagePrefixes: make(map[string]string),

		resolved: make(map[Dependency]bool),
	}
	deps.rootConfig = cfg
	dep := Dependency{Protocol: "path", Repository: cfg.Package.Root}
	deps.canonicalPaths[dep] = cfg.Package.Name
	deps.usedImports[cfg.Package.Name] = true
	deps.addPackage(cfg)
	return deps
}

type resolveResult struct {
	dep Dependency
	err error
}

func (deps *DependencyTracker) ResolveAll() error {
	defer debug.TimedFunction(time.Now(), "DependencyTracker.ResolveAll()")

	var results []chan resolveResult
	for len(deps.unresolved) > 0 || len(results) > 0 {

		// Start a go routine for each dependency
		for _, dep := range deps.unresolved {
			if !deps.resolved[dep] {
				deps.resolved[dep] = true

				// Lookup arguments for the resolver
				importPath := deps.canonicalPaths[dep]
				resolver, exists := builtinResolvers[dep.Protocol]
				if !exists {
					// FIXME: Cancel or wait for any running go-routines
					return fmt.Errorf(`No resolver exists for protocol "%s"`, dep.Protocol)
				}

				// Download the dependency asynchronously
				result := make(chan resolveResult)
				results = append(results, result)
				go func(ch chan resolveResult, fn ResolverFunc, dep Dependency, path string) {
					err := fn(dep.Repository, path, deps.rootConfig.Workspace)
					ch <- resolveResult{dep: dep, err: err}
				}(result, resolver, dep, importPath)
			}
		}
		deps.unresolved = nil

		// Add the resolved dependencies in the same order as they were added to the
		// unresolved list, to maintain a deterministic dependency graph.
		if len(results) > 0 {

			// Always wait for at least one package to be resolved
			{
				result := <-results[0]
				results = results[1:]
				if result.err == AlreadyResolved {
					// Do nothing!
				} else if result.err != nil {
					return result.err // FIXME: Cancel or wait for any running go-routines
				} else {
					deps.loadPackage(result.dep)
				}
			}

			// Also load as many other packages as are ready (in-order, w/o skipping)
			resultsCompleted := 0
		loopResults:
			for _, ch := range results {
				select {
				case result := <-ch: // if this result is ready
					resultsCompleted += 1

					if result.err == AlreadyResolved {
						// Do nothing!
					} else if result.err != nil {
						return result.err // FIXME: Cancel or wait for any running go-routines
					} else {
						deps.loadPackage(result.dep)
					}

				default: // if this results is NOT ready
					break loopResults
				}
			}
			results = results[resultsCompleted:]
		}
	}

	// Use the go tool to fetch non-Bottle dependencies with "go get" without
	// adding any more branches to the dependency tree
	for _, importPath := range deps.needsFallback {
		start := time.Now()

		cmd := shutil.Cmd(`go`, `get`, `-d`, importPath)
		cmd.Env = append([]string{"GOPATH=" + deps.rootConfig.Workspace}, cmd.Env...)
		cmd.Dir = deps.rootConfig.Workspace
		output, err := cmd.Try()
		if err != nil {
			tabbedOutput := strings.Join(strings.Split(output, "\n"), "\n\t")
			return fmt.Errorf("Failed fetching \"%s\" dependencies with \"go get\":\n\n\t%s\n", importPath, tabbedOutput)
		}

		debug.TimedFunction(start, "block { go get -d "+importPath+" }")
	}

	return nil
}

func (deps *DependencyTracker) InstallAll() error {
	defer debug.TimedFunction(time.Now(), "DependencyTracker.InstallAll()")

	shutil.MkdirParents(deps.rootConfig.Workspace+"/bin", 0755)
	for packagePath := range deps.installPackages {
		if deps.updatedImports[deps.packagePrefixes[packagePath]] {
			exePath := shutil.Path(deps.rootConfig.Workspace, "/bin", path.Base(packagePath))
			cmd := shutil.Cmd(`go`, `build`, `-o`, exePath, packagePath)
			cmd.Env = append([]string{"GOPATH=" + deps.rootConfig.Workspace}, cmd.Env...)
			cmd.Dir = deps.rootConfig.Workspace
			output, err := cmd.Try()
			if err != nil {
				tabbedOutput := strings.Join(strings.Split(output, "\n"), "\n\t")
				return fmt.Errorf("Failed to build dependency:\n\n\t%s\n", tabbedOutput)
			}
		}
	}

	return nil
}

func (deps *DependencyTracker) loadPackage(dep Dependency) {
	importPath := deps.canonicalPaths[dep]
	deps.updatedImports[importPath] = true

	// Find the new package's dependencies
	dest := shutil.Path(deps.rootConfig.Workspace, "src", importPath)
	cfg, err := discoverPackage(dest, shutil.Path(dest, "Bottle.toml"), false)
	if err != nil {
		log.Fatalf("Failed to find package in \"%s\":\n\n\t%s\n\n", dest, err.Error())
	}

	// HACK: This overrides the project directory set by discoverPackage in
	//       order to properly handle recursive "path" dependencies.
	if dep.Protocol == "path" {
		cfg.Project = dep.Repository
	}

	{
		//
		// TODO / FIXME : Apply import renames
		//
	}

	deps.addPackage(cfg)
}

func (deps *DependencyTracker) addPackage(cfg *Config) {
	if cfg.Missing {
		if !shutil.Exists(shutil.Path(cfg.Package.Root, "vendor")) {
			deps.needsFallback = append(deps.needsFallback, cfg.ImportPath+"/...")
		}
	}

	// TODO: maybe sort config dependencies before iterating?
	for importPath, meta := range cfg.Dependencies {
		packagePath := importPath

		// Parse (and normalize) the package's source from the config
		var dep Dependency
		switch {
		case len(meta.Path) > 0:
			dep.Repository = shutil.Abspath(shutil.Path(cfg.Project, meta.Path))
			dep.Protocol = "path"
		case len(meta.Git) > 0:
			dep.Repository = meta.Git
			dep.Protocol = "git"
		default:
			if strings.HasPrefix(importPath, "bitbucket.org") {
				importPath = parseImportPrefix(importPath)
				dep.Repository = "https://" + importPath + ".git"
				dep.Protocol = "git"
			} else if strings.HasPrefix(importPath, "github.com") {
				importPath = parseImportPrefix(importPath)
				dep.Repository = "https://" + importPath + ".git"
				dep.Protocol = "git"
			} else {
				dep.Repository = importPath
				dep.Protocol = "go-get"
			}
		}

		// Skip the package if we've already added it
		if canonical, ok := deps.canonicalPaths[dep]; ok {
			if meta.Install {
				deps.installPackages[canonical] = true
			}
			continue
		}

		// Check that this import path isn't already in use
		if deps.usedImports[importPath] {
			// TODO: return an error, with advice on how to fix the problem
			panic("FIXME: handle duplicate import paths (" + importPath + ")")
		}

		// Add a new unresolved dependency and register this import path as the canonical path
		deps.unresolved = append(deps.unresolved, dep)
		deps.canonicalPaths[dep] = importPath
		deps.usedImports[importPath] = true
		if meta.Install {
			deps.installPackages[packagePath] = true
			deps.packagePrefixes[packagePath] = importPath
		}
	}
}

func parseImportPrefix(importPath string) string {
	prefix := importPath
	prefixParts := strings.Split(prefix, "/")
	if len(prefixParts) >= 3 { // NOTE: otherwise just use the path and let an error occur later
		prefix = strings.Join([]string{prefixParts[0], prefixParts[1], prefixParts[2]}, "/")
	}
	return prefix
}
