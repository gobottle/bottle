package shutil

import (
	"bytes"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

var Args = os.Args

func Env() map[string]string {
	vars := os.Environ()
	env := make(map[string]string, len(vars))
	for _, v := range vars {
		if len(v) > 0 {
			keyValue := strings.SplitN(v, "=", 2)
			env[keyValue[0]] = keyValue[1]
		}
	}
	return env
}

type Command struct{ *exec.Cmd }

func Cmd(path string, args ...string) Command {
	cmd := exec.Command(path, args...)
	cmd.Env = os.Environ()
	return Command{Cmd: cmd}
}

func (cmd Command) Run() string {
	var buf bytes.Buffer
	cmd.Stdout = &buf
	cmd.Stderr = os.Stderr
	err := cmd.Cmd.Run()
	if err != nil {
		panic(fmt.Errorf("%s: %s", cmd.Path, err))
	}
	return buf.String()
}

func (cmd Command) Try() (string, error) {
	var buf bytes.Buffer
	cmd.Stdout = &buf
	cmd.Stderr = &buf
	err := cmd.Cmd.Run()
	return buf.String(), err
}

func (cmd Command) Bind() error {
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Cmd.Run()
}

func Must(err error) {
	if err != nil {
		panic(err)
	}
}

func Exit(errCode int) {
	os.Exit(errCode)
}

func Echo(args ...interface{}) {
	fmt.Println(args...)
}

func Stdout(args ...string) {
	fmt.Fprint(os.Stdout, strings.Join(args, " "))
}

func Stderr(args ...string) {
	fmt.Fprint(os.Stderr, strings.Join(args, " "))
}

func Path(parts ...string) string {
	return filepath.Join(parts...)
}

func Cd(path string) string {
	err := os.Chdir(path)
	if err != nil {
		panic(err)
	}
	return path
}

func Pwd() string {
	path, err := os.Getwd()
	if err != nil {
		panic(err)
	}
	return path
}

func Binread(path string) []byte {
	return ReadBytes(path)
}

func Read(path string) string {
	b, err := ioutil.ReadFile(path)
	if err != nil {
		panic(err)
	}
	return string(b)
}

func ReadBytes(path string) []byte {
	b, err := ioutil.ReadFile(path)
	if err != nil {
		panic(err)
	}
	return b
}

func Mkdir(path string, mode os.FileMode) {
	err := os.Mkdir(path, mode)
	if err != nil {
		panic(err)
	}
}

func MkdirParents(path string, mode os.FileMode) {
	err := os.MkdirAll(path, mode)
	if err != nil {
		panic(err)
	}
}

func Cp(src, dst string) {
	if !IsRegularFile(src) {
		if IsDirectory(src) {
			panic("cp: cannot copy directory '" + src + "'; not a recursive copy")
		} else {
			panic("cp: cannot copy '" + src + "'; no such file or directory")
		}
	}

	if IsDirectory(dst) {
		dst = Path(dst, filepath.Base(src))
	}

	srcio, err := os.Open(src)
	if err != nil {
		panic(err)
	}
	defer srcio.Close()

	dstio, err := os.Create(dst)
	if err != nil {
		panic(err)
	}
	defer dstio.Close()

	_, err = io.Copy(dstio, srcio)
	if err != nil {
		panic(err)
	}

	err = dstio.Sync()
	if err != nil {
		panic(err)
	}

	stat, err := srcio.Stat()
	if err != nil {
		panic(err)
	}

	err = dstio.Chmod(stat.Mode())
	if err != nil {
		panic(err)
	}
}

func Mv(src, dst string) {
	err := os.Rename(src, dst)
	if err != nil {
		panic(err)
	}
}

func Rm(path string) {
	if path == "/" {
		panic("rm: removal of / is not allowed")
	}

	err := os.Remove(path)
	if err != nil {
		panic(err)
	}
}

func RmRecursive(path string) {
	if path == "/" {
		panic("rm: removal of / is not allowed")
	}

	err := os.RemoveAll(path)
	if err != nil {
		panic(err)
	}
}

func Rmdir(path string) {
	if IsDirectory(path) {
		Rm(path)
	} else {
		panic(fmt.Errorf("rmdir: failed to remove '%s': not a directory", path))
	}
}

func Dirname(path string) string {
	return filepath.Dir(path)
}

func Abspath(path string) string {
	abs, err := filepath.Abs(path)
	if err != nil {
		panic(err)
	}
	return abs
}

func Relpath(from, to string) string {
	rel, err := filepath.Rel(from, to)
	if err != nil {
		panic(err)
	}
	return rel
}

func IsSubdir(child, parent string) bool {
	var err error
	child, err = filepath.Abs(child)
	if err != nil {
		panic(err)
	}
	parent, err = filepath.Abs(parent)
	if err != nil {
		panic(err)
	}

	rel, err := filepath.Rel(parent, child)
	if err != nil {
		return false
	}
	return !strings.Contains(rel, "..")
}

func Exists(path string) bool {
	_, err := os.Stat(path)
	if os.IsNotExist(err) {
		return false
	}
	if err != nil {
		panic(err)
	}
	return true
}

func IsDirectory(path string) bool {
	file, err := os.Stat(path)
	if os.IsNotExist(err) {
		return false
	}
	if err != nil {
		panic(err)
	}
	return file.Mode().IsDir()
}

func IsRegularFile(path string) bool {
	file, err := os.Stat(path)
	if os.IsNotExist(err) {
		return false
	}
	if err != nil {
		panic(err)
	}
	return file.Mode().IsRegular()
}

func Ls(path string) []string {
	files := LsLong(path)
	names := make([]string, len(files))
	for i, file := range files {
		names[i] = file.Name()
	}
	return names
}

func LsLong(path string) []os.FileInfo {
	files, err := ioutil.ReadDir(path)
	if err != nil {
		panic(err)
	}
	return files
}
