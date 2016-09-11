package stitch

import (
	"fmt"
	"golang.org/x/tools/go/vcs"
	"os"
	"os/user"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spf13/afero"

	"github.com/NetSys/quilt/util"
)

func TestGetQuiltPath(t *testing.T) {
	os.Setenv(QuiltPathKey, "")
	actual := GetQuiltPath()
	usr, err := user.Current()
	if err != nil {
		t.Error(err)
	}
	expected := filepath.Join(usr.HomeDir, ".quilt")
	if actual != expected {
		t.Errorf("expected %s \n but got %s", expected, actual)
	}
}

// Modify the download and create functions so that calls are not made to the network.
// They will update a list of directories accessed, to verify behavior of checkSpec and
// downloadSpec.
var updated []string
var created []string

func initVCSFunc() {
	updated = []string{}
	created = []string{}

	download = func(repo *vcs.RepoRoot, dir string) error {
		updated = append(updated, dir)
		return nil
	}

	create = func(repo *vcs.RepoRoot, dir string) error {
		created = append(created, dir)
		return nil
	}
}

func testCheckSpec(t *testing.T) {
	initVCSFunc()
	getter := ImportGetter{
		Path: ".",
	}
	util.AppFs = afero.NewMemMapFs()
	util.AppFs.Mkdir("test", 777)
	util.WriteFile("test/noDownload.js",
		[]byte(`require("nextimport/nextimport")`), 0644)
	util.AppFs.Mkdir("nextimport", 777)
	util.WriteFile("nextimport/nextimport.js", []byte("dummy = 1;"), 0644)
	if err := getter.checkSpec("test/noDownload.js", nil, nil); err != nil {
		t.Error(err)
	}

	if len(created) != 0 {
		t.Errorf("should not have downloaded, but downloaded %s", created)
	}

	// Verify that call is made to GetSpec
	util.WriteFile("test/toDownload.js",
		[]byte(`require("github.com/NetSys/quilt/specs/example")`), 0644)
	expected := "StitchError: unable to open import " +
		"github.com/NetSys/quilt/specs/example " +
		"(path=github.com/NetSys/quilt/specs/example.js)"
	err := getter.checkSpec("test/toDownload.js", nil, nil)
	if !strings.HasPrefix(err.Error(), expected) {
		t.Errorf("'%s' does not begin with '%s'", err.Error(), expected)
	}

	if len(created) == 0 {
		t.Error("did not download dependency!")
	}

	expected = "github.com/NetSys/quilt"
	if created[0] != expected {
		t.Errorf("expected to download %s \n but got %s", expected, created[0])
	}
}

func TestResolveSpecImports(t *testing.T) {
	testCheckSpec(t)
	initVCSFunc()
	getter := ImportGetter{
		Path: ".",
	}
	// This will error because we do not actually download the file. Checking a spec
	// is handled in testCheckSpec.
	expected := "StitchError: unable to open import " +
		"github.com/NetSys/quilt/specs/example " +
		"(path=github.com/NetSys/quilt/specs/example.js)"
	if err := getter.resolveSpecImports("test"); err.Error() != expected {
		t.Errorf("Resolve error didn't match: expected %q, got %q.",
			expected, err.Error())
	}

	if len(created) == 0 {
		t.Error("did not download dependency!")
	}

	expected = "github.com/NetSys/quilt"
	if created[0] != expected {
		t.Errorf("expected to download %s \n but got %s", expected, created[0])
	}
}

// Due to limitations with git and afero, GetSpec will error at resolveSpecImports.
// Instead, resolveSpecImports is covered above.
func TestGetSpec(t *testing.T) {
	initVCSFunc()
	getter := ImportGetter{
		Path: "./getspecs",
	}
	util.AppFs = afero.NewMemMapFs()
	util.AppFs.Mkdir("getspecs", 777)
	importPath := "github.com/NetSys/quilt"

	// Clone
	if _, err := getter.downloadSpec(importPath); err != nil {
		t.Error(err)
	}

	if len(created) == 0 {
		t.Errorf("did not download dependency %s", importPath)
	}

	expected := "getspecs/github.com/NetSys/quilt"
	if created[0] != expected {
		t.Errorf("expected to download %s \n but got %s", expected, created[0])
	}
	fmt.Println(created)
	util.AppFs.Mkdir("getspecs/github.com/NetSys/quilt", 777)

	// Update
	if _, err := getter.downloadSpec(importPath); err != nil {
		t.Error(err)
	}

	if len(updated) == 0 {
		t.Errorf("did not update dependency %s", importPath)
	}

	expected = "getspecs/github.com/NetSys/quilt"
	if updated[0] != expected {
		t.Errorf("expected to update %s \n but got %s", expected, updated[0])
	}
}
