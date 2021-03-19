package main

import (
	"bufio"
	"bytes"
	"flag"
	"html/template"
	"io"
	"os"
	"os/exec"
	"path"
	"path/filepath"
)

func main() {
	opts := DefaultOptions()
	opts.ParseFlags(os.Args[1:])

	if err := genPackages(opts); err != nil {
		panic(err)
	}
}

type Package struct {
	Source      string
	Module      string
	ImportPath  string
	Description string
}

type Options struct {
	Config string
	Source string
	Clean  bool
	Output string
}

func DefaultOptions() *Options {
	return &Options{
		Config: ".vanitic",
		Source: filepath.Join(os.TempDir(), "vanitic"),
		Clean:  false,
		Output: "pkg",
	}
}

func (opts *Options) ParseFlags(args []string) error {
	fset := flag.NewFlagSet("vanitic", flag.ExitOnError)

	fset.StringVar(
		&opts.Config, "c", opts.Config,
		"Configuration file path.",
	)

	fset.StringVar(
		&opts.Source, "src", opts.Source,
		"Directory where packages source code live.",
	)

	fset.BoolVar(
		&opts.Clean, "clean", opts.Clean,
		"Remove output directory before generating files.",
	)

	fset.StringVar(
		&opts.Output, "out", opts.Output,
		"Directory where Go packages HTML files will be written.",
	)

	if err := fset.Parse(args); err != nil {
		return err
	}

	return opts.Validate()
}

func (opts *Options) Validate() error {
	opts.Config = filepath.Clean(opts.Config)
	opts.Source = filepath.Clean(opts.Source)
	opts.Output = filepath.Clean(opts.Output)

	return nil
}

func cloneRepo(dst, src string) error {
	if dst == "" {
		dst = path.Base(src)
	}

	if _, err := os.Stat(dst); err == nil {
		return runCmd(dst, "git", "pull", "origin", "master")
	}

	if err := runCmd(".", "git", "clone", src, dst); err != nil {
		if err := os.RemoveAll(dst); err != nil {
			return err
		}

		return err
	}

	return nil
}

func getReposURL(configFile string) ([]string, error) {
	f, err := os.Open(configFile)
	if err != nil {
		return nil, err
	}

	defer f.Close()
	reposURL := []string{}
	s := bufio.NewScanner(f)

	for s.Scan() {
		reposURL = append(reposURL, s.Text())
	}

	if err := s.Err(); err != nil {
		return nil, err
	}

	return reposURL, nil
}

func genPackages(opts *Options) error {
	if opts.Clean {
		if err := os.RemoveAll(opts.Output); err != nil {
			return err
		}
	}

	if err := os.Mkdir(opts.Output, 0755); err != nil && !os.IsExist(err) {
		return err
	}

	reposURL, err := getReposURL(opts.Config)
	if err != nil {
		return err
	}

	for _, repoURL := range reposURL {
		name := path.Base(repoURL)
		repo := filepath.Join(opts.Source, name)

		if err := cloneRepo(repo, repoURL); err != nil {
			return err
		}

		output, err := runCmdOutput(repo, "go", "list", "-m")
		if err != nil {
			return err
		}

		pkg := Package{}
		pkg.Source = repoURL
		pkg.Module = string(bytes.TrimSpace(output))
		pkg.ImportPath = pkg.Module
		dst := filepath.Join(opts.Output, pkg.ImportPath, "index.html")

		if err := writePackage(dst, pkg); err != nil {
			return err
		}

		output, err = runCmdOutput(repo, "go", "list",
			"-f", "{{ .ImportPath }} {{ .Doc }}",
			"./...",
		)

		if err != nil {
			return err
		}

		for _, entry := range bytes.Split(bytes.TrimSpace(output), []byte{'\n'}) {
			x := bytes.SplitN(entry, []byte{' '}, 2)
			pkg.ImportPath, pkg.Description = string(x[0]), string(x[1])
			dst := filepath.Join(opts.Output, pkg.ImportPath, "index.html")

			if err := writePackage(dst, pkg); err != nil {
				return err
			}
		}
	}

	return nil
}

func runCmd(dir string, args ...string) error {
	return runCmdWrite(os.Stdout, dir, args...)
}

func runCmdOutput(dir string, args ...string) ([]byte, error) {
	buf := bytes.NewBuffer(nil)
	err := runCmdWrite(buf, dir, args...)

	return buf.Bytes(), err
}

func runCmdWrite(w io.Writer, dir string, args ...string) error {
	c := exec.Command(args[0], args[1:]...)
	c.Stdout = w
	c.Stderr = os.Stderr
	c.Dir = dir

	return c.Run()
}

func writePackage(dst string, pkg Package) error {
	if err := os.MkdirAll(filepath.Dir(dst), 0755); err != nil {
		return err
	}

	f, err := os.Create(dst)
	if err != nil {
		return err
	}

	defer f.Close()

	return goPkgTmpl.Execute(f, pkg)
}

var goPkgTmpl = template.Must(template.New("package").Parse(`<!DOCTYPE html>
<html>
<head>
  <meta http-equiv="Content-Type" content="text/html; charset=utf-8"/>
  <meta name="go-import" content="{{ .Module }} git {{ .Source }}"/>
  <meta name="go-source" content="{{ .Module }} {{ .Source }} {{ .Source }}/tree/master{/dir} {{ .Source }}/blob/master{/dir}/{file}#L{line}"/>
</head>
<body>
  <h1>{{ .ImportPath }}</h1>
  <p>{{ .Description }}</p>
  <p><a href="https://pkg.go.dev/{{ .ImportPath }}/">See the package documentation.</a></p>
</body>
</html>
`))
