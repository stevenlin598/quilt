package stitch

import (
	"bufio"
	"errors"
	"fmt"
	"golang.org/x/tools/go/vcs"
	"os"
	"os/user"
	"path/filepath"
	"text/scanner"

	"github.com/NetSys/quilt/util"

	log "github.com/Sirupsen/logrus"

	"github.com/spf13/afero"
)

// ImportGetter provides functions for working with imports.
type ImportGetter struct {
	Path         string
	AutoDownload bool
}

// DefaultImportGetter uses the default QUILT_PATH, and doesn't automatically
// download imports.
var DefaultImportGetter = ImportGetter{
	Path: GetQuiltPath(),
}

// GetQuiltPath returns the user-defined QUILT_PATH, or the default absolute QUILT_PATH,
// which is ~/.quilt if the user did not specify a QUILT_PATH.
func GetQuiltPath() string {
	quiltPath := os.Getenv("QUILT_PATH")
	if quiltPath != "" {
		return quiltPath
	}

	usr, err := user.Current()
	if err != nil {
		log.WithError(err).
			Error("Unable to get current user to generate QUILT_PATH")
		return ""
	}

	quiltPath = filepath.Join(usr.HomeDir, ".quilt")
	return quiltPath
}

// Break out the download and create functions for unit testing
var download = func(repo *vcs.RepoRoot, dir string) error {
	return repo.VCS.Download(dir)
}

var create = func(repo *vcs.RepoRoot, dir string) error {
	return repo.VCS.Create(dir, repo.Repo)
}

// Download takes in an import path `repoName`, and attempts to download the
// repository associated with that repoName.
func (getter ImportGetter) Download(repoName string) error {
	path, err := getter.downloadSpec(repoName)
	if err != nil {
		return err
	}
	return getter.resolveSpecImports(path)
}

func (getter ImportGetter) downloadSpec(repoName string) (string, error) {
	repo, err := vcs.RepoRootForImportPath(repoName, true)
	if err != nil {
		return "", err
	}

	path := filepath.Join(getter.Path, repo.Root)
	if _, err := util.AppFs.Stat(path); os.IsNotExist(err) {
		log.Info(fmt.Sprintf("Cloning %s into %s", repo.Root, path))
		if err := create(repo, path); err != nil {
			return "", err
		}
	} else {
		log.Info(fmt.Sprintf("Updating %s in %s", repo.Root, path))
		download(repo, path)
	}
	return path, nil
}

func (getter ImportGetter) resolveSpecImports(folder string) error {
	return afero.Walk(util.AppFs, folder, getter.checkSpec)
}

func (getter ImportGetter) checkSpec(file string, _ os.FileInfo, _ error) error {
	if filepath.Ext(file) != ".spec" {
		return nil
	}
	f, err := util.Open(file)

	if err != nil {
		return err
	}
	defer f.Close()

	sc := scanner.Scanner{
		Position: scanner.Position{
			Filename: file,
		},
	}
	_, err = Compile(*sc.Init(bufio.NewReader(f)),
		ImportGetter{
			Path:         getter.Path,
			AutoDownload: true,
		})
	return err
}

func (getter ImportGetter) get(name string) (scanner.Scanner, error) {
	modulePath := filepath.Join(getter.Path, name+".spec")
	if _, err := os.Stat(modulePath); os.IsNotExist(err) && getter.AutoDownload {
		getter.Download(name)
	}

	var sc scanner.Scanner
	f, err := util.Open(modulePath)
	if err != nil {
		return sc, fmt.Errorf("unable to open import %s (path=%s)",
			name, modulePath)
	}
	sc.Filename = modulePath
	sc.Init(bufio.NewReader(f))
	return sc, nil
}

func (getter ImportGetter) resolveImports(asts []ast) ([]ast, error) {
	return getter.resolveImportsRec(asts, nil)
}

func (getter ImportGetter) resolveImportsRec(
	asts []ast, imported []string) ([]ast, error) {

	var newAsts []ast
	top := true // Imports are required to be at the top of the file.

	for _, ast := range asts {
		name := parseImport(ast)
		if name == "" {
			newAsts = append(newAsts, ast)
			top = false
			continue
		}

		if !top {
			return nil, errors.New(
				"import must be at the beginning of the module")
		}

		// Check for any import cycles.
		for _, importedModule := range imported {
			if name == importedModule {
				return nil, fmt.Errorf("import cycle: %s",
					append(imported, name))
			}
		}

		moduleScanner, err := getter.get(name)
		if err != nil {
			return nil, err
		}

		parsed, err := parse(moduleScanner)
		if err != nil {
			return nil, err
		}

		// Rename module name to last name in import path
		name = filepath.Base(name)
		parsed, err = getter.resolveImportsRec(parsed, append(imported, name))
		if err != nil {
			return nil, err
		}

		module := astModule{body: parsed, moduleName: astString(name)}
		newAsts = append(newAsts, module)
	}

	return newAsts, nil
}

func parseImport(ast ast) string {
	sexp, ok := ast.(astSexp)
	if !ok {
		return ""
	}

	if len(sexp.sexp) < 1 {
		return ""
	}

	fnName, ok := sexp.sexp[0].(astBuiltIn)
	if !ok {
		return ""
	}

	if fnName != "import" {
		return ""
	}

	name, ok := sexp.sexp[1].(astString)
	if !ok {
		return ""
	}

	return string(name)
}
