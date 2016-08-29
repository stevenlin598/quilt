package specs

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/robertkrimen/otto"

	"github.com/NetSys/quilt/stitch"
)

func configRunOnce(configPath string, quiltPath string) error {
	stitch.GetGithubKeys = func(username string) ([]string, error) {
		return nil, nil
	}
	_, err := stitch.Compile(configPath, stitch.ImportGetter{
		Path: quiltPath,
	})
	return err
}

func TestConfigs(t *testing.T) {
	t.Skip("Vivian will convert the specs")
	testConfig := func(configPath string, quiltPath string) {
		if err := configRunOnce(configPath, quiltPath); err != nil {
			errString := err.Error()
			// Print the stacktrace if it's an Otto error.
			if ottoError, ok := err.(*otto.Error); ok {
				errString = ottoError.String()
			}
			t.Errorf("%s failed validation: %s \n quiltPath: %s",
				configPath, errString, quiltPath)
		}
	}

	goPath := os.Getenv("GOPATH")
	quiltPath := filepath.Join(goPath, "src")

	testConfig("example.spec", "specs/stdlib")
	testConfig("../quilt-tester/config/infrastructure.spec", quiltPath)
	testConfig("../quilt-tester/tests/basic/basic.spec", quiltPath)
	testConfig("../quilt-tester/tests/spark/spark.spec", quiltPath)
	testConfig("./spark/sparkPI.spec", quiltPath)
	testConfig("./wordpress/main.spec", quiltPath)
	testConfig("./etcd/example.spec", quiltPath)
	testConfig("./redis/example.spec", quiltPath)
}
