package main

import (
	"bufio"
	"fmt"
	"io/ioutil"
	"os"
	"path"
	"path/filepath"
	"strings"
	"time"

	"bottle/debug"
	sh "bottle/shutil"
)

type BuildFlags struct{ ldflags, outfile, outdir string }

func buildProject(cfg *Config, pwd string, flags BuildFlags) {
	defer debug.TimedFunction(time.Now(), "buildProject()")

	//  Build the project
	buildArgs := []string{`install`}
	if len(flags.ldflags) > 0 {
		buildArgs = append(buildArgs, "-ldflags", flags.ldflags)
	}

	targetDir := sh.Path(cfg.Workspace, "src", cfg.Package.Name)
	if sh.Exists(sh.Path(targetDir, "vendor")) {
		buildArgs = append(buildArgs, `.`) // TODO: detect non-vendor packages and build those
	} else {
		buildArgs = append(buildArgs, `./...`)
	}

	cmd := sh.Cmd(`go`, buildArgs...)
	cmd.Env = append([]string{"GOPATH=" + cfg.Workspace}, cmd.Env...)
	cmd.Dir = targetDir
	output, err := cmd.Try()
	output = strings.Replace(output, targetDir, ".", -1)
	if err != nil {
		sh.Stderr(output)
		sh.Exit(1)
	}

	/* Copy the outputs to the output file/directory */
	binprefix := cfg.Workspace + "/bin/"
	if len(flags.outfile) > 0 {
		if !filepath.IsAbs(flags.outfile) {
			flags.outfile = sh.Path(pwd, flags.outfile)
		}

		outdir := path.Dir(flags.outfile)
		if !sh.Exists(outdir) {
			sh.MkdirParents(outdir, 0755)
		}

		if len(cfg.Bin) > 0 {
			bincfg := cfg.Bin[0]
			bindir := path.Dir(bincfg.Path)
			if len(bindir) > 0 && bindir != "." {
				sh.Cp(binprefix+path.Base(bindir), flags.outfile)
			} else {
				sh.Cp(binprefix+bincfg.Name, flags.outfile)
			}
		} else if sh.IsRegularFile(cfg.Workspace + "/bin/" + cfg.Package.Name) {
			sh.Cp(binprefix+cfg.Package.Name, flags.outfile)
		} else {
			sh.Stderr("bottle: no output exists that can be written to a file\n")
			sh.Exit(1)
		}
	} else if len(flags.outdir) > 0 {
		if !filepath.IsAbs(flags.outdir) {
			flags.outdir = sh.Path(pwd, flags.outdir)
		}

		if !sh.Exists(flags.outdir) {
			sh.MkdirParents(flags.outdir, 0755)
		}

		for _, bincfg := range cfg.Bin {
			bindir := path.Dir(bincfg.Path)
			if len(bindir) > 0 && bindir != "." {
				sh.Cp(binprefix+path.Base(bindir), sh.Path(flags.outdir, bincfg.Name))
			} else {
				sh.Cp(binprefix+bincfg.Name, sh.Path(flags.outdir, bincfg.Name))
			}
		}
	}
}

func execTool(cfg *Config, tool string, args []string) {
	defer debug.TimedFunction(time.Now(), "execTool()")

	workdir := sh.Path(cfg.Workspace, "src", cfg.Package.Name)
	cmd := sh.Cmd(tool, args...)
	cmd.Env = append(cmd.Env, "GOPATH="+cfg.Workspace)
	cmd.Dir = workdir
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	err := cmd.Bind()
	if err != nil {
		sh.Exit(1)
	}

	if !cfg.Missing {
		sh.Cmd(`rsync`, `-rum`, workdir+"/", cfg.Package.Root).Run()
	}
}

type PublishFlags struct{ release string }

func publishProject(cfg *Config, flags PublishFlags) {
	defer debug.TimedFunction(time.Now(), "publishProject()")

	// FIXME: the publish behavior is way too opinionated and special case
	//        and it needs to be generalized for regular use

	// Check the configuration
	if !cfg.Package.Publish {
		sh.Stderr("error: can't publish project, publish hasn't been set to true")
		sh.Exit(1)
	}
	if len(cfg.Package.Repository) == 0 {
		sh.Stderr("error: can't publish project, no repository specified")
		sh.Exit(1)
	}

	// Clone the public repository
	pubdir := sh.Path(os.TempDir(), "bottle", ".publish", cfg.Package.Name)
	if sh.Exists(pubdir) {
		sh.RmRecursive(pubdir)
	}
	sh.MkdirParents(pubdir, 0755)
	output, err := sh.Cmd(`git`, `clone`, `--no-checkout`, cfg.Package.Repository, pubdir).Try()
	if err != nil {
		tabbedOutput := strings.Join(strings.Split(output, "\n"), "\n\t")
		sh.Stderr(fmt.Sprintf("error: can't clone public repository:\n\n\t%s\n", tabbedOutput))
		sh.Exit(1)
	}

	// Ensure the public repository is up to date
	cmd := sh.Cmd(`git`, `pull`)
	cmd.Dir = pubdir
	output, err = cmd.Try()
	if err != nil {
		tabbedOutput := strings.Join(strings.Split(output, "\n"), "\n\t")
		sh.Stderr(fmt.Sprintf("error: can't update public repository:\n\n\t%s\n", tabbedOutput))
		sh.Exit(1)
	}

	// Copy the changes into the public repository
	sh.Cmd(`rsync`, `-rum`, cfg.Project+"/", pubdir).Run()

	// Stage the changes in the repository
	cmd = sh.Cmd(`git`, `add`, `.`)
	cmd.Dir = pubdir
	output, err = cmd.Try()
	if err != nil {
		tabbedOutput := strings.Join(strings.Split(output, "\n"), "\n\t")
		sh.Stderr(fmt.Sprintf("error: can't stage changes:\n\n\t%s\n", tabbedOutput))
		sh.Exit(1)
	}

	// Create a new branch
	sh.Stdout("Branch: ")
	reader := bufio.NewReader(os.Stdin)
	branch, _ := reader.ReadString('\n')
	branch = branch[:len(branch)-1]

	cmd = sh.Cmd(`git`, `checkout`, `-b`, branch)
	cmd.Dir = pubdir
	output, err = cmd.Try()
	if err != nil {
		tabbedOutput := strings.Join(strings.Split(output, "\n"), "\n\t")
		sh.Stderr(fmt.Sprintf("error: can't create branch:\n\n\t%s\n", tabbedOutput))
		sh.Exit(1)
	}

	// Create a new commit
	sh.Stdout("Subject: ")
	subject, _ := reader.ReadString('\n')

	sh.Stdout("Description: (end on newline with Ctrl+D)\n")
	descBytes, _ := ioutil.ReadAll(os.Stdin)
	description := string(descBytes)
	sh.Stdout("\n")

	message := fmt.Sprintf("%s\n%s", subject, description)
	cmd = sh.Cmd(`git`, `commit`, `-m`, message)
	cmd.Dir = pubdir
	output, err = cmd.Try()
	if err != nil {
		tabbedOutput := strings.Join(strings.Split(output, "\n"), "\n\t")
		sh.Stderr(fmt.Sprintf("error: can't create commit:\n\n\t%s\n", tabbedOutput))
		sh.Exit(1)
	}

	// Push to remote host
	cmd = sh.Cmd(`git`, `push`, `-u`, `origin`, branch)
	cmd.Dir = pubdir
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	err = cmd.Bind()
	if err != nil {
		sh.Exit(1)
	}

	// Tell user to open a new Pull Request
	sh.Stdout("\nOpen a Pull Request for this branch\n\n")
	sh.Stdout("\t" + cfg.Package.Repository + "/compare/" + branch + "?expand=1\n\n")
}
