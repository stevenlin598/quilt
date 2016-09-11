package stitch

import (
	"fmt"
	"golang.org/x/tools/go/vcs"
	"os"
	"os/user"
	"path/filepath"

	"github.com/NetSys/quilt/util"

	log "github.com/Sirupsen/logrus"

	"github.com/robertkrimen/otto"
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

// QuiltPathKey is the environment variable key we use to lookup the Quilt path.
const QuiltPathKey = "QUILT_PATH"

// GetQuiltPath returns the user-defined QUILT_PATH, or the default absolute QUILT_PATH,
// which is ~/.quilt if the user did not specify a QUILT_PATH.
func GetQuiltPath() string {
	quiltPath := os.Getenv(QuiltPathKey)
	if quiltPath != "" {
		return quiltPath
	}

	usr, err := user.Current()
	if err != nil {
		// XXX: Figure out proper way to handle this error. Current isn't
		// implemented on linux/amd64, so this always errors on the minion.
		log.WithError(err).
			Errorf("Unable to get current user to generate %s", QuiltPathKey)
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
	if filepath.Ext(file) != ".js" {
		return nil
	}
	_, err := Compile(file,
		ImportGetter{
			Path:         getter.Path,
			AutoDownload: true,
		})
	return err
}

func (getter ImportGetter) get(name string) (string, error) {
	modulePath := filepath.Join(getter.Path, name+".js")
	if _, err := os.Stat(modulePath); os.IsNotExist(err) && getter.AutoDownload {
		getter.Download(name)
	}

	spec, err := util.ReadFile(modulePath)
	if err != nil {
		return "", fmt.Errorf("unable to open import %s (path=%s)",
			name, modulePath)
	}
	return spec, nil
}

type importSources map[string]string

const importSourcesKey = "importSources"

// XXX: Error on import cycles.
func (getter ImportGetter) requireImpl(call otto.FunctionCall) otto.Value {
	if len(call.ArgumentList) < 1 {
		panic(call.Otto.MakeRangeError(
			"require requires the import as an argument"))
	}
	name, err := call.Argument(0).ToString()
	if err != nil {
		panic(err)
	}

	vm := call.Otto
	imports, err := getImports(vm)
	if err != nil {
		panic(err)
	}

	impStr, ok := imports[name]
	if !ok {
		impStr, err = getter.get(name)
		if err != nil {
			stitchError(vm, err)
		}
		if err := setImport(vm, name, impStr); err != nil {
			panic(err)
		}
	}

	// The function declaration must be prepended to the first line of the
	// import or else stacktraces will show an offset line number.
	exec := "(function() {" +
		"var module={exports: {}};" +
		"(function(module, exports) {" +
		impStr +
		"})(module, module.exports);" +
		"return module.exports" +
		"})()"
	script, err := vm.Compile(name, exec)
	if err != nil {
		panic(err)
	}

	module, err := vm.Run(script)
	if err != nil {
		panic(err)
	}
	return module
}

func setImport(vm *otto.Otto, moduleName, moduleContents string) error {
	imports, getImportsErr := getImports(vm)
	if getImportsErr != nil {
		// If this is the first import we're setting, the map won't exist yet.
		imports = make(map[string]string)
	}
	imports[moduleName] = moduleContents
	importSourcesVal, err := vm.ToValue(imports)
	if err != nil {
		return assertOttoError(err)
	}
	return assertOttoError(vm.Set(importSourcesKey, importSourcesVal))
}

func getImports(vm *otto.Otto) (importSources, error) {
	imports := make(map[string]string)
	importsVal, err := vm.Get(importSourcesKey)
	if err != nil {
		return imports, assertOttoError(err)
	}

	if importsVal.IsUndefined() {
		return imports, nil
	}

	err = forField(importsVal,
		func(importName string, importVal otto.Value) (err error) {
			imports[importName], err = importVal.ToString()
			return err
		})
	return imports, assertOttoError(err)
}

func (imports importSources) String() string {
	importSourcesStr := "{"
	for impName, impSrc := range imports {
		importSourcesStr += fmt.Sprintf("%q: %q,",
			impName, impSrc)
	}
	importSourcesStr += "}"
	return importSourcesStr
}
