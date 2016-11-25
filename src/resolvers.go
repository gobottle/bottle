package main

import (
	"fmt"
	"io/ioutil"
	"net/http"
	"regexp"
	"strings"
	"sync"
	"time"

	"bottle/debug"
	"bottle/shutil"
)

type ResolverFunc func(src string, pkg string, workspace string) error
type ResolverList map[string]ResolverFunc

var AlreadyResolved = fmt.Errorf("Return this error if the package has already been resolved")

var builtinResolvers = ResolverList{
	"go-get": goGetResolver,
	"git":    gitResolver,
	"path":   pathResolver,
}

var goImportMetaTag = regexp.MustCompile(`<meta\s[^>]*name\s*=\s*("go-import"|'go-import'|go-import)[^>]*>`)
var goImportMetaContent = regexp.MustCompile(`content\s*=\s*("[^\s"]+ [^\s"]+ [^\s"]+"|'[^\s']+ [^\s']+ [^\s']+')`)

type goImportMeta struct{ prefix, vcs, repo string }

// See https://golang.org/cmd/go/#hdr-Remote_import_paths
func goGetResolver(src string, pkg string, workspace string) error {
	defer debug.TimedFunction(time.Now(), "goGetResolver("+src+")")

	// Don't re-resolve the package if we already have it
	if shutil.Exists(shutil.Path(workspace, "src", pkg)) {
		return AlreadyResolved
	}

	// Use HTTP to fetch the package metadata
	meta, err := goGetMeta(src)
	if err != nil {
		return err
	}
	if meta.prefix != src {
		parentMeta, err := goGetMeta(src)
		if err != nil {
			return err
		}
		if *parentMeta != *meta {
			return fmt.Errorf(`resolver: go-import meta for "%s" does not match its prefix "%s"`, src, meta.prefix)
		}
	}

	// Resolve the package with the appropriate VCS
	switch meta.vcs {
	case "git":
		return gitResolver(meta.repo, meta.prefix, workspace)
	default:
		return fmt.Errorf(`resolver: unknown VCS "%s" when resolving remote import "%s"`, meta.vcs, src)
	}
}
func goGetMeta(prefix string) (*goImportMeta, error) {
	url := "https://" + prefix + "?go-get=1"
	resp, err := http.Get(url)
	if err != nil {
		return nil, fmt.Errorf(`resolver: %s`, err)
	}
	if resp.StatusCode != 200 {
		return nil, fmt.Errorf(`resolver: recevied status %d from "%s", expecting 200`, url)
	}

	data, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf(`resolver: error reading package metadata from "%s"`, url)
	}

	// IMPORTANT: Parsing HTML with regular expressions is definitely the best idea!!!
	// NOTE: Because I prefer to avoid adding any external dependencies right now
	importTagBytes := goImportMetaTag.Find(data)
	if len(importTagBytes) == 0 {
		return nil, fmt.Errorf(`resolver: did not find "go-import" meta in response from "%s"`, url)
	}
	contentAttrMatches := goImportMetaContent.FindSubmatch(importTagBytes)
	if len(contentAttrMatches) < 1 {
		return nil, fmt.Errorf(`resolver: remote import meta for "%s" does not contain correctly formatted content`, prefix)
	}
	parts := strings.Split(string(contentAttrMatches[1][1:len(contentAttrMatches[1])-1]), " ")
	return &goImportMeta{prefix: parts[0], vcs: parts[1], repo: parts[2]}, nil
}

var gitMutex sync.Mutex
var gitMap = make(map[string]*sync.Cond)

func gitResolver(src string, pkg string, workspace string) error {
	defer debug.TimedFunction(time.Now(), "gitResolver("+src+")")

	dest := shutil.Path(workspace, "src", pkg)

	// Prevent multiple gitResolvers from cloning into the same directory concurrently
	{
		gitMutex.Lock()
		if cond, ok := gitMap[dest]; ok {
			gitMutex.Unlock()
			cond.Wait()
		} else {
			mutex := new(sync.Mutex)
			mutex.Lock()
			gitMap[dest] = sync.NewCond(mutex)
			gitMutex.Unlock()

			defer func() {
				gitMutex.Lock()

				gitMap[dest].Broadcast()
				delete(gitMap, dest)

				gitMutex.Unlock()
			}()
		}
	}

	// Don't re-clone the repository if it already exists
	if shutil.Exists(dest) {
		if !shutil.Exists(shutil.Path(dest, ".git")) {
			return fmt.Errorf("While resolving %s, directory \"%s\" exists but is not a Git repository", pkg, dest)
		}

		// FIXME: Check to see if we are on the correct commit-ish
		//        If not, do a "clean" checkout of the correct commit
		return AlreadyResolved
	}

	// Actually clone the repository
	output, err := shutil.Cmd(`git`, `clone`, src, dest).Try()
	if err != nil {
		tabbedOutput := strings.Join(strings.Split(output, "\n"), "\n\t")
		return fmt.Errorf("resolver: error cloning repository with git:\n\n\t%s\n", tabbedOutput)
	}

	return nil
}

func pathResolver(src string, pkg string, workspace string) error {
	defer debug.TimedFunction(time.Now(), "pathResolver("+src+")")

	// TODO: maybe implement file sync in Go
	dest := shutil.Path(workspace, "src", pkg)
	if !shutil.Exists(dest) {
		shutil.MkdirParents(dest, 0755)
	}

	args := []string{`-rum`, `--exclude`, `.*`, `--delete`, src + "/", dest}
	output, err := shutil.Cmd(`rsync`, args...).Try()
	if err != nil {
		tabbedOutput := strings.Join(strings.Split(output, "\n"), "\n\t")
		return fmt.Errorf("resolver: error copying package with rsync:\n\n\t%s\n", tabbedOutput)
	}

	return nil
}
