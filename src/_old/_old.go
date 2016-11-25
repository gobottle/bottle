package old

import (
	"bufio"
	"io"
	"os"
	"strings"

	sh "bottle/shutil"
)

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

	if !opts.Dryrun {
		if !sh.Exists(dest) {
			sh.MkdirParents(dest, 0755)
		}
	}

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
